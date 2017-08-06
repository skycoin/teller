package exchange

import (
	"errors"
	"sync"
	"time"

	"fmt"

	"encoding/binary"
	"encoding/json"

	"github.com/boltdb/bolt"
)

var (
	// exchange meta info bucket
	exchangeMetaBkt = []byte("exchange_meta")

	// deposit status bucket
	depositInfoBkt = []byte("deposit_info")

	// bind address bucket
	bindAddressBkt = []byte("bind_address")

	// index bucket for skycoin address and deposit seqs, skycoin address as key
	// deposit info seq array as value
	skyDepositSeqsIndexBkt = []byte("sky_deposit_seqs_index")
)

func createBucketFailedErr(name []byte, err error) error {
	return fmt.Errorf("Create bucket %s failed: %v", string(name), err)
}

func bucketNotExistErr(name []byte) error {
	return fmt.Errorf("Bucket %s doesn't exist", name)
}

// store storage for exchange
type store struct {
	db    *bolt.DB
	cache *cache
}

// newStore creates a store instance
func newStore(db *bolt.DB) (*store, error) {
	if db == nil {
		return nil, errors.New("new exchange store failed, db is nil")
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		// create exchange meta bucket if not exist
		if _, err := tx.CreateBucketIfNotExists(exchangeMetaBkt); err != nil {
			return createBucketFailedErr(exchangeMetaBkt, err)
		}

		// create deposit status bucket if not exist
		if _, err := tx.CreateBucketIfNotExists(depositInfoBkt); err != nil {
			return createBucketFailedErr(depositInfoBkt, err)
		}

		// create bind address bucket if not exist
		if _, err := tx.CreateBucketIfNotExists(bindAddressBkt); err != nil {
			return createBucketFailedErr(bindAddressBkt, err)
		}

		if _, err := tx.CreateBucketIfNotExists(skyDepositSeqsIndexBkt); err != nil {
			return createBucketFailedErr(skyDepositSeqsIndexBkt, err)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	// load cache from db
	cache, err := loadCache(db)
	if err != nil {
		return nil, err
	}

	return &store{
		db:    db,
		cache: cache,
	}, nil
}

// GetBindAddress returns binded skycoin address of given bitcoin address
func (s *store) GetBindAddress(btcAddr string) (string, bool) {
	return s.cache.getBindAddr(btcAddr)
}

// AddDepositInfo adds deposit info into storage, return seq or error
func (s *store) AddDepositInfo(dpinfo DepositInfo) (uint64, error) {
	// verify the deposit info
	// verify if the skycoin address is empty, we will use it
	// to create seq index
	if dpinfo.SkyAddress == "" {
		return 0, errors.New("skycoin address is empty")
	}

	if err := s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(depositInfoBkt)
		if bkt == nil {
			return bucketNotExistErr(depositInfoBkt)
		}

		// dpinfo.Seq, _ = bkt.NextSequence()
		dpinfo.UpdatedAt = time.Now().UTC().Unix()

		// update index of skycoin address and the deposit seq
		skyIndexBkt := tx.Bucket(skyDepositSeqsIndexBkt)
		if skyIndexBkt == nil {
			return bucketNotExistErr(skyDepositSeqsIndexBkt)
		}

		var addrs []string
		addrsBytes := skyIndexBkt.Get([]byte(dpinfo.SkyAddress))
		if addrsBytes != nil {
			if err := json.Unmarshal(addrsBytes, &addrs); err != nil {
				return err
			}
		}

		dpinfo.Seq = uint64(len(addrs))

		// write deposit info back to db
		v, err := json.Marshal(dpinfo)
		if err != nil {
			return err
		}

		if err := bkt.Put([]byte(dpinfo.BtcAddress), v); err != nil {
			return err
		}

		// add btc address to bind list
		addrs = append(addrs, dpinfo.BtcAddress)
		v, err = json.Marshal(addrs)
		if err != nil {
			return err
		}

		if err := skyIndexBkt.Put([]byte(dpinfo.SkyAddress), v); err != nil {
			return err
		}

		// bind address
		bindAddrBkt := tx.Bucket(bindAddressBkt)
		if bindAddrBkt == nil {
			return bucketNotExistErr(bindAddressBkt)
		}

		if v := bindAddrBkt.Get([]byte(dpinfo.BtcAddress)); v != nil {
			return fmt.Errorf("address %s already binded", dpinfo.BtcAddress)
		}

		if err := bindAddrBkt.Put([]byte(dpinfo.BtcAddress),
			[]byte(dpinfo.SkyAddress)); err != nil {
			return fmt.Errorf("Bind address failed: %v", err)
		}

		// update the caches
		s.cache.setDepositInfo(dpinfo)
		s.cache.addSkyIndex(dpinfo.SkyAddress, dpinfo.BtcAddress)
		s.cache.setBindAddr(dpinfo.BtcAddress, dpinfo.SkyAddress)

		return nil
	}); err != nil {
		return 0, err
	}

	return dpinfo.Seq, nil
}

// GetDepositInfo returns depsoit info of given btc address
func (s *store) GetDepositInfo(btcAddr string) (DepositInfo, bool) {
	return s.cache.getDepositInfo(btcAddr)
}

// GetAllDepositInfo returns all deposit info
func (s *store) GetDepositInfoArray(flt DepositFilter) []DepositInfo {
	return s.cache.getDepositInfoArray(flt)
}

// UpdateDepositStatus updates deposit info
func (s *store) UpdateDepositInfo(btcAddr string, updateFunc func(DepositInfo) DepositInfo) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(depositInfoBkt)
		if bkt == nil {
			return bucketNotExistErr(depositInfoBkt)
		}

		v := bkt.Get([]byte(btcAddr))
		if v == nil {
			return fmt.Errorf("DepositInfo of btc address %s doesn't exist in db", btcAddr)
		}

		var dpi DepositInfo
		if err := json.Unmarshal(v, &dpi); err != nil {
			return err
		}

		dpiNew := updateFunc(dpi)
		dpiNew.UpdatedAt = time.Now().UTC().Unix()

		dpi.updateMutableVar(dpiNew)

		d, err := json.Marshal(dpi)
		if err != nil {
			return err
		}

		if err := bkt.Put([]byte(btcAddr), d); err != nil {
			return err
		}

		// update deposit info cache
		s.cache.setDepositInfo(dpi)

		return nil
	})
}

// GetDepositInfoOfSkyAddress returns all deposit info that are binded
// to the given skycoin address
func (s *store) GetDepositInfoOfSkyAddress(skyAddr string) ([]DepositInfo, error) {
	btcAddrs := s.cache.getSkyIndex(skyAddr)
	dpis := make([]DepositInfo, 0, len(btcAddrs))
	for _, btcAddr := range btcAddrs {
		dpi, ok := s.cache.getDepositInfo(btcAddr)
		if !ok {
			return []DepositInfo{}, fmt.Errorf("Get deposit info of btc addr %s from cache failed", btcAddr)
		}

		dpis = append(dpis, dpi)
	}

	return dpis, nil
}

// GetSkyBindBtcAddresses returns the btc addresses of the given sky address binded
func (s *store) GetSkyBindBtcAddresses(skyAddr string) []string {
	return s.cache.getSkyIndex(skyAddr)
}

type cache struct {
	sync.RWMutex
	depositInfo         map[string]DepositInfo
	skyDepositSeqsIndex map[string][]string
	bindAddress         map[string]string
}

func loadCache(db *bolt.DB) (*cache, error) {
	c := cache{
		depositInfo:         make(map[string]DepositInfo),
		skyDepositSeqsIndex: make(map[string][]string),
		bindAddress:         make(map[string]string),
	}

	if err := db.View(func(tx *bolt.Tx) error {
		// init deposit info cache
		dpiBkt := tx.Bucket(depositInfoBkt)
		if dpiBkt == nil {
			return bucketNotExistErr(depositInfoBkt)
		}

		if err := dpiBkt.ForEach(func(k, v []byte) error {
			btcAddr := string(k)
			var dpi DepositInfo
			if err := json.Unmarshal(v, &dpi); err != nil {
				return err
			}

			c.depositInfo[btcAddr] = dpi
			return nil
		}); err != nil {
			return err
		}

		// init sky index cache
		skyIndexBkt := tx.Bucket(skyDepositSeqsIndexBkt)
		if skyIndexBkt == nil {
			return bucketNotExistErr(skyDepositSeqsIndexBkt)
		}

		if err := skyIndexBkt.ForEach(func(k, v []byte) error {
			skyAddr := string(k)
			var btcAddrs []string
			if err := json.Unmarshal(v, &btcAddrs); err != nil {
				return err
			}
			c.skyDepositSeqsIndex[skyAddr] = btcAddrs
			return nil
		}); err != nil {
			return err
		}

		// init bind address cache
		bindAddrBkt := tx.Bucket(bindAddressBkt)
		if bindAddrBkt == nil {
			return bucketNotExistErr(bindAddressBkt)
		}

		return bindAddrBkt.ForEach(func(k, v []byte) error {
			btcAddr := string(k)
			skyAddr := string(v)
			c.bindAddress[btcAddr] = skyAddr
			return nil
		})
	}); err != nil {
		return nil, err
	}

	return &c, nil
}

func (c *cache) setBindAddr(btcAddr, skyAddr string) {
	c.Lock()
	c.bindAddress[btcAddr] = skyAddr
	c.Unlock()
}

func (c *cache) getBindAddr(btcAddr string) (string, bool) {
	c.RLock()
	c.RUnlock()
	skyAddr, ok := c.bindAddress[btcAddr]
	return skyAddr, ok
}

func (c *cache) setDepositInfo(dpi DepositInfo) {
	c.Lock()
	c.depositInfo[dpi.BtcAddress] = dpi
	c.Unlock()
}

func (c *cache) getDepositInfo(btcAddr string) (DepositInfo, bool) {
	c.RLock()
	defer c.RUnlock()
	dpi, ok := c.depositInfo[btcAddr]
	return dpi, ok
}

func (c *cache) getDepositInfoArray(flt DepositFilter) []DepositInfo {
	c.RLock()
	defer c.RUnlock()
	dpis := make([]DepositInfo, 0, len(c.depositInfo))
	for _, dpi := range c.depositInfo {
		if flt(dpi) {
			dpis = append(dpis, dpi)
		}
	}
	return dpis
}

func (c *cache) addSkyIndex(skyAddr string, btcAddr string) {
	c.Lock()
	c.Unlock()
	if btcAddrs, ok := c.skyDepositSeqsIndex[skyAddr]; ok {
		btcAddrs = append(btcAddrs, btcAddr)
		c.skyDepositSeqsIndex[skyAddr] = btcAddrs
		return
	}

	c.skyDepositSeqsIndex[skyAddr] = []string{btcAddr}
}

func (c *cache) getSkyIndex(skyAddr string) []string {
	c.RLock()
	c.RUnlock()
	return c.skyDepositSeqsIndex[skyAddr]
}

func uint64ToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	return b
}

func bytesToUint64(b []byte) uint64 {
	return binary.LittleEndian.Uint64(b)
}
