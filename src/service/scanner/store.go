package scanner

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/boltdb/bolt"
)

var (
	scanMetaBkt         = []byte("scan_meta")
	lastScanBlockKey    = []byte("last_scan_block")
	depositAddressesKey = []byte("deposit_addresses")
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
		_, err := tx.CreateBucketIfNotExists(scanMetaBkt)
		if err != nil {
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
				s.cache.addDepositAddress(a)
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

func (s *store) getDepositAddresses() []string {
	return s.cache.getDepositAddreses()
}

func (s *store) addDepositAddress(addr string) error {
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

		s.cache.addDepositAddress(addr)
		return nil
	})
}

func bucketNotExistErr(bktName []byte) error {
	return fmt.Errorf("%s bucket does not exist", string(bktName))
}

func dupDepositAddrErr(addr string) error {
	return fmt.Errorf("deposit address %s already exist", addr)
}

type cache struct {
	sync.RWMutex
	scanAddresses map[string]struct{}
	lastScanBlock lastScanBlock
}

func newCache() *cache {
	return &cache{
		scanAddresses: make(map[string]struct{}),
	}
}

func (c *cache) addDepositAddress(addr string) {
	c.Lock()
	c.scanAddresses[addr] = struct{}{}
	c.Unlock()
}

func (c *cache) getDepositAddreses() []string {
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
