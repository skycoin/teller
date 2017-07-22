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

type cache struct {
	sync.RWMutex
	depositInfo         map[uint64]depositInfo
	skyDepositSeqsIndex map[string][]uint64
	bindAddress         map[string]string
}

// newStore creates a store instance
func newStore(db *bolt.DB) (*store, error) {
	if db == nil {
		return nil, errors.New("New exchange store failed, db is nil")
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

// BindAddress binds deposit bitcoin address and skycoin address
// func (s *store) BindAddress(btcAddr, skyAddr string) error {
// 	return s.db.Update(func(tx *bolt.Tx) error {
// 		bkt := tx.Bucket(bindAddressBkt)
// 		if bkt == nil {
// 			return fmt.Errorf("Bind address failed: bucket %s doesn't exist", bindAddressBkt)
// 		}

// 		if v := bkt.Get([]byte(btcAddr)); v != nil {
// 			return fmt.Errorf("address %s already binded", btcAddr)
// 		}

// 		if err := bkt.Put([]byte(btcAddr), []byte(skyAddr)); err != nil {
// 			return fmt.Errorf("Bind address failed: %v", err)
// 		}

// 		s.cache.setBindAddr(btcAddr, skyAddr)
// 		return nil
// 	})
// }

// GetBindAddress returns binded skycoin address of given bitcoin address
func (s *store) GetBindAddress(btcAddr string) (string, bool) {
	return s.cache.getBindAddr(btcAddr)
}

// AddDepositInfo adds deposit info into storage, return seq or error
func (s *store) AddDepositInfo(dpinfo depositInfo) (uint64, error) {
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

		// get seq
		dpinfo.Seq, _ = bkt.NextSequence()
		dpinfo.UpdatedAt = time.Now().UTC().Unix()

		v, err := json.Marshal(dpinfo)
		if err != nil {
			return err
		}

		if err := bkt.Put(uint64ToBytes(dpinfo.Seq), v); err != nil {
			return err
		}

		// update index of skycoin address and the deposit seq
		skyIndexBkt := tx.Bucket(skyDepositSeqsIndexBkt)
		if skyIndexBkt == nil {
			return bucketNotExistErr(skyDepositSeqsIndexBkt)
		}

		var seqs []uint64
		seqsBytes := skyIndexBkt.Get([]byte(dpinfo.SkyAddress))
		if seqsBytes != nil {
			if err := json.Unmarshal(seqsBytes, &seqs); err != nil {
				return err
			}
		}

		seqs = append(seqs, dpinfo.Seq)
		v, err = json.Marshal(seqs)
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
		s.cache.addSkyIndex(dpinfo.SkyAddress, dpinfo.Seq)
		s.cache.setBindAddr(dpinfo.BtcAddress, dpinfo.SkyAddress)

		return nil
	}); err != nil {
		return 0, err
	}

	return dpinfo.Seq, nil
}

// UpdateDepositStatus updates deposit info
func (s *store) UpdateDepositStatus(seq uint64, st status) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(depositInfoBkt)
		if bkt == nil {
			return bucketNotExistErr(depositInfoBkt)
		}

		v := bkt.Get(uint64ToBytes(seq))
		if v == nil {
			return fmt.Errorf("DepositInfo of seq %d doesn't exist in db", seq)
		}

		var dpi depositInfo
		if err := json.Unmarshal(v, &dpi); err != nil {
			return err
		}

		if dpi.Seq != seq {
			return fmt.Errorf("Get deposit info of seq %d from db"+
				", but the seq in returned deposit info is %d", seq, dpi.Seq)
		}

		dpi.Status = st
		dpi.UpdatedAt = time.Now().UTC().Unix()

		d, err := json.Marshal(dpi)
		if err != nil {
			return err
		}

		if err := bkt.Put(uint64ToBytes(seq), d); err != nil {
			return err
		}

		// update deposit info cache
		s.cache.setDepositInfo(dpi)

		return nil
	})
}

// GetDepositInfoOfSkyAddress returns all deposit info that are binded
// to the given skycoin address
func (s *store) GetDepositInfoOfSkyAddress(skyAddr string) ([]depositInfo, error) {
	seqs := s.cache.getSkyIndex(skyAddr)
	dpis := make([]depositInfo, 0, len(seqs))
	for _, seq := range seqs {
		dpi, ok := s.cache.getDepositInfo(seq)
		if !ok {
			return []depositInfo{}, fmt.Errorf("Get deposit info of seq %d from cache failed", seq)
		}

		dpis = append(dpis, dpi)
	}

	return dpis, nil
}

func loadCache(db *bolt.DB) (*cache, error) {
	c := cache{
		depositInfo:         make(map[uint64]depositInfo),
		skyDepositSeqsIndex: make(map[string][]uint64),
		bindAddress:         make(map[string]string),
	}

	if err := db.View(func(tx *bolt.Tx) error {
		// init deposit info cache
		dpiBkt := tx.Bucket(depositInfoBkt)
		if dpiBkt == nil {
			return bucketNotExistErr(depositInfoBkt)
		}

		if err := dpiBkt.ForEach(func(k, v []byte) error {
			seq := bytesToUint64(k)
			var dpi depositInfo
			if err := json.Unmarshal(v, &dpi); err != nil {
				return err
			}

			c.depositInfo[seq] = dpi
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
			var seqs []uint64
			if err := json.Unmarshal(v, &seqs); err != nil {
				return err
			}
			c.skyDepositSeqsIndex[skyAddr] = seqs
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

func (c *cache) setDepositInfo(dpi depositInfo) {
	c.Lock()
	c.depositInfo[dpi.Seq] = dpi
	c.Unlock()
}

func (c *cache) getDepositInfo(seq uint64) (depositInfo, bool) {
	c.RLock()
	defer c.RUnlock()
	dpi, ok := c.depositInfo[seq]
	return dpi, ok
}

func (c *cache) addSkyIndex(skyAddr string, seq uint64) {
	c.Lock()
	c.Unlock()
	if seqs, ok := c.skyDepositSeqsIndex[skyAddr]; ok {
		seqs = append(seqs, seq)
		c.skyDepositSeqsIndex[skyAddr] = seqs
		return
	}

	c.skyDepositSeqsIndex[skyAddr] = []uint64{seq}
}

func (c *cache) getSkyIndex(skyAddr string) []uint64 {
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
