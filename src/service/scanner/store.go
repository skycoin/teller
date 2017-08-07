package scanner

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/boltdb/bolt"
)

var (
	scanMetaBkt     = []byte("scan_meta")
	depositValueBkt = []byte("deposit_value")

	lastScanBlockKey    = []byte("last_scan_block")
	depositAddressesKey = []byte("deposit_addresses")
	dvIndexListKey      = []byte("dv_index_list") // deposit value index list
)

var (
	ErrDepositValueExist = errors.New("deposit value already exist")
)

// store records scanner meta info
type store struct {
	db    *bolt.DB
	cache *cache
}

func newStore(db *bolt.DB) (*store, error) {
	if db == nil {
		return nil, errors.New("new store failed: db is nil")
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		// create lastScanBlock bucket if not exist
		if _, err := tx.CreateBucketIfNotExists(scanMetaBkt); err != nil {
			return err
		}

		if _, err := tx.CreateBucketIfNotExists(depositValueBkt); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return nil, err
	}

	s := &store{
		db:    db,
		cache: newCache(),
	}

	if err := s.loadCache(); err != nil {
		return nil, fmt.Errorf("load cache failed: %v", err)
	}

	return s, nil
}

// lastScanBlock struct in bucket
type lastScanBlock struct {
	Hash   string
	Height int64
}

func (s *store) loadCache() error {
	return s.db.View(func(tx *bolt.Tx) error {
		// load last scanblock from db
		metaBkt := tx.Bucket(scanMetaBkt)
		if metaBkt == nil {
			return bucketNotExistErr(lastScanBlockKey)
		}

		var lsb lastScanBlock
		if v := metaBkt.Get(lastScanBlockKey); v != nil {
			if err := json.Unmarshal(v, &lsb); err != nil {
				return err
			}
		}

		s.cache.setLastScanBlock(lsb)

		// load scan addresses
		var addrs []string
		if v := metaBkt.Get(depositAddressesKey); v != nil {
			if err := json.Unmarshal(v, &addrs); err != nil {
				return err
			}
			for _, a := range addrs {
				s.cache.addScanAddress(a)
			}
		}

		dvBkt := tx.Bucket(depositValueBkt)
		if dvBkt == nil {
			return bucketNotExistErr(depositValueBkt)
		}

		if iv := metaBkt.Get(dvIndexListKey); iv != nil {
			var idxs []string
			if err := json.Unmarshal(iv, &idxs); err != nil {
				return err
			}

			for _, idx := range idxs {
				dvb := dvBkt.Get([]byte(idx))
				if dvb == nil {
					return fmt.Errorf("deposit value of %s doesn't exist in db", idx)
				}
				var dv DepositValue
				if err := json.Unmarshal(dvb, &dv); err != nil {
					return err
				}

				s.cache.pushDepositValue(dv)
			}
		}

		return nil
	})
}

// getLastScanBlock returns the last scanned block hash and height
func (s *store) getLastScanBlock() (string, int64, error) {
	lsb := s.cache.getLastScanBlock()
	return lsb.Hash, lsb.Height, nil
}

func (s *store) setLastScanBlock(b lastScanBlock) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(scanMetaBkt)
		if bkt == nil {
			return bucketNotExistErr(lastScanBlockKey)
		}
		v, err := json.Marshal(b)
		if err != nil {
			return err
		}

		if err := bkt.Put(lastScanBlockKey, v); err != nil {
			return err
		}

		s.cache.setLastScanBlock(b)
		return nil
	})
}

func (s *store) getScanAddresses() []string {
	return s.cache.getScanAddreses()
}

func (s *store) addScanAddress(addr string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(scanMetaBkt)
		if bkt == nil {
			return bucketNotExistErr(depositAddressesKey)
		}
		var addrs []string
		if v := bkt.Get(depositAddressesKey); v != nil {
			if err := json.Unmarshal(v, &addrs); err != nil {
				return err
			}
		}
		for _, a := range addrs {
			if a == addr {
				return dupDepositAddrErr(addr)
			}
		}

		addrs = append(addrs, addr)
		v, err := json.Marshal(addrs)
		if err != nil {
			return err
		}

		if err := bkt.Put(depositAddressesKey, v); err != nil {
			return err
		}

		s.cache.addScanAddress(addr)
		return nil
	})
}

func (s *store) removeScanAddr(addr string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		// remove scan address from db
		var addrs []string
		if err := getBktValue(tx, scanMetaBkt, depositAddressesKey, &addrs); err != nil {
			return err
		}

		addrMap := make(map[string]int)
		for i, a := range addrs {
			addrMap[a] = i
		}

		if idx, ok := addrMap[addr]; ok {
			addrs = append(addrs[:idx], addrs[idx+1:]...)
			// write back to db
			if err := putBktValue(tx, scanMetaBkt, depositAddressesKey, addrs); err != nil {
				return err
			}
		}

		s.cache.removeScanAddr(addr)

		return nil
	})
}

func (s *store) getHeadDepositValue() (DepositValue, bool) {
	return s.cache.getHeadDepositValue()
}

func (s *store) pushDepositValue(dv DepositValue) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		// persist deposit value
		dvBkt := tx.Bucket(depositValueBkt)
		if dvBkt == nil {
			return bucketNotExistErr(depositValueBkt)
		}

		key := fmt.Sprintf("%v:%v", dv.Tx, dv.N)
		// check if already exist
		if v := dvBkt.Get([]byte(key)); v != nil {
			return ErrDepositValueExist
		}

		v, err := json.Marshal(dv)
		if err != nil {
			return err
		}

		if err := dvBkt.Put([]byte(key), v); err != nil {
			return err
		}

		// update deposit value index
		metaBkt := tx.Bucket(scanMetaBkt)
		if metaBkt == nil {
			return bucketNotExistErr(scanMetaBkt)
		}

		var index []string
		if iv := metaBkt.Get(dvIndexListKey); iv != nil {
			if err := json.Unmarshal(iv, &index); err != nil {
				return err
			}
		}

		index = append(index, key)
		ivx, err := json.Marshal(index)
		if err != nil {
			return err
		}

		if err := metaBkt.Put(dvIndexListKey, ivx); err != nil {
			return err
		}

		// update cache
		s.cache.pushDepositValue(dv)

		return nil
	})
}

func (s *store) popDepositValue() (DepositValue, bool, error) {
	var dv DepositValue
	var ok bool
	if err := s.db.Update(func(tx *bolt.Tx) error {
		metaBkt := tx.Bucket(scanMetaBkt)
		if metaBkt == nil {
			return bucketNotExistErr(scanMetaBkt)
		}

		v := metaBkt.Get(dvIndexListKey)
		if v == nil {
			return nil
		}

		var index []string
		if err := json.Unmarshal(v, &index); err != nil {
			return err
		}

		if len(index) == 0 {
			ok = false
			return nil
		}

		head := index[0]
		index = index[1:]
		// write index back to db
		if err := putBktValue(tx, scanMetaBkt, dvIndexListKey, index); err != nil {
			return err
		}

		// mark deposit value in bucket as used
		if err := getBktValue(tx, depositValueBkt, []byte(head), &dv); err != nil {
			return err
		}

		dv.IsUsed = true

		if err := putBktValue(tx, depositValueBkt, []byte(head), dv); err != nil {
			return err
		}

		dv, ok = s.cache.popDepositValue()
		if !ok {
			return errors.New("pop deposit value failed, it's empty")
		}

		if head != fmt.Sprintf("%v:%v", dv.Tx, dv.N) {
			return errors.New("head deposit value in cache is different from db")
		}

		return nil
	}); err != nil {
		return DepositValue{}, false, err
	}

	return dv, ok, nil
}

func bucketNotExistErr(bktName []byte) error {
	return fmt.Errorf("%s bucket does not exist", string(bktName))
}

func dupDepositAddrErr(addr string) error {
	return fmt.Errorf("deposit address %s already exist", addr)
}

func getBktValue(tx *bolt.Tx, bktName []byte, key []byte, value interface{}) error {
	refV := reflect.ValueOf(value)
	if refV.Kind() != reflect.Ptr {
		return fmt.Errorf("value is not setable")
	}

	bkt := tx.Bucket(bktName)
	if bkt == nil {
		return bucketNotExistErr(bktName)
	}

	v := bkt.Get(key)
	if v == nil {
		return fmt.Errorf("value of key %v does not exist in bucket %v", string(key), string(bktName))
	}

	if err := json.Unmarshal(v, value); err != nil {
		return fmt.Errorf("decode value failed: %v", err)
	}

	return nil
}

func putBktValue(tx *bolt.Tx, bktName []byte, key []byte, value interface{}) error {
	bkt := tx.Bucket(bktName)
	if bkt == nil {
		return bucketNotExistErr(bktName)
	}
	v, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode value failed: %v", err)
	}

	return bkt.Put(key, v)
}

type cache struct {
	sync.RWMutex
	scanAddresses map[string]struct{}
	lastScanBlock lastScanBlock
	depositValues []DepositValue
}

func newCache() *cache {
	return &cache{
		scanAddresses: make(map[string]struct{}),
	}
}

func (c *cache) addScanAddress(addr string) {
	c.Lock()
	c.scanAddresses[addr] = struct{}{}
	c.Unlock()
}

func (c *cache) getScanAddreses() []string {
	c.RLock()
	defer c.RUnlock()
	var addrs []string
	for addr := range c.scanAddresses {
		addrs = append(addrs, addr)
	}
	return addrs
}

func (c *cache) setLastScanBlock(lsb lastScanBlock) {
	c.Lock()
	c.lastScanBlock = lsb
	c.Unlock()
}

func (c *cache) getLastScanBlock() lastScanBlock {
	c.RLock()
	defer c.RUnlock()
	return c.lastScanBlock
}

func (c *cache) pushDepositValue(dv DepositValue) {
	c.Lock()
	c.depositValues = append(c.depositValues, dv)
	c.Unlock()
}

func (c *cache) popDepositValue() (DepositValue, bool) {
	c.Lock()
	defer c.Unlock()
	if len(c.depositValues) == 0 {
		return DepositValue{}, false
	}

	dv := c.depositValues[0]
	c.depositValues = c.depositValues[1:]
	return dv, true
}

func (c *cache) getHeadDepositValue() (DepositValue, bool) {
	c.RLock()
	defer c.RUnlock()
	if len(c.depositValues) == 0 {
		return DepositValue{}, false
	}

	return c.depositValues[0], true
}

func (c *cache) removeScanAddr(addr string) {
	c.Lock()
	delete(c.scanAddresses, addr)
	c.Unlock()
}
