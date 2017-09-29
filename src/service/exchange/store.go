package exchange

import (
	"errors"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"

	"fmt"

	"encoding/binary"
	"encoding/json"

	"sort"

	"github.com/boltdb/bolt"
)

var (
	// exchange meta info bucket
	exchangeMetaBkt = []byte("exchange_meta")

	// deposit status bucket
	depositInfoBkt = []byte("deposit_info")

	// bind address bucket
	bindAddressBkt = []byte("bind_address")

	btcTxsBkt = []byte("btc_txs")

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

		if _, err := tx.CreateBucketIfNotExists(btcTxsBkt); err != nil {
			return createBucketFailedErr(btcTxsBkt, err)
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

func (s *store) BindAddress(skyaddr, btcaddr string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		// update index of skycoin address and the deposit seq
		var addrs []string
		_, err := getBktValue(tx, skyDepositSeqsIndexBkt, []byte(skyaddr), &addrs)
		if err != nil {
			return err
		}

		addrs = append(addrs, btcaddr)
		if err := putBktValue(tx, skyDepositSeqsIndexBkt, []byte(skyaddr), addrs); err != nil {
			return err
		}

		if err := putBktValue(tx, bindAddressBkt, []byte(btcaddr), skyaddr); err != nil {
			return err
		}

		s.cache.addSkyIndex(skyaddr, btcaddr)
		s.cache.setBindAddr(btcaddr, skyaddr)
		return nil
	})
}

func isValidBtcTx(btcTx string) bool {
	if btcTx == "" {
		return false
	}

	pts := strings.Split(btcTx, ":")
	if len(pts) != 2 {
		return false
	}

	if pts[0] == "" {
		return false
	}

	return true
}

// AddDepositInfo adds deposit info into storage, return seq or error
func (s *store) AddDepositInfo(dpinfo DepositInfo) error {
	if !isValidBtcTx(dpinfo.BtcTx) {
		log.Println("Invalid dpinfo.BtcTx:", dpinfo.BtcTx)
		return fmt.Errorf("btc txid \"%s\" is empty/invalid", dpinfo.BtcTx)
	}

	if dpinfo.BtcAddress == "" {
		return errors.New("btc address is empty")
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		// check if the dpi with btctx already exist
		bkt := tx.Bucket(depositInfoBkt)
		if bkt == nil {
			return bucketNotExistErr(depositInfoBkt)
		}

		var dpi DepositInfo
		ok, err := getBktValue(tx, depositInfoBkt, []byte(dpinfo.BtcTx), &dpi)
		if err != nil {
			return err
		}

		if ok {
			return fmt.Errorf("deposit info of btctx %s already exist", dpinfo.BtcTx)
		}

		dpinfo.Seq, _ = bkt.NextSequence()
		dpinfo.UpdatedAt = time.Now().UTC().Unix()

		if err := putBktValue(tx, depositInfoBkt, []byte(dpinfo.BtcTx), dpinfo); err != nil {
			return err
		}

		// update btc_txids bucket
		var txs []string
		_, err = getBktValue(tx, btcTxsBkt, []byte(dpinfo.BtcAddress), &txs)
		if err != nil {
			return err
		}

		txs = append(txs, dpinfo.BtcTx)
		if err := putBktValue(tx, btcTxsBkt, []byte(dpinfo.BtcAddress), txs); err != nil {
			return err
		}

		s.cache.setDepositInfo(dpinfo)
		s.cache.addBtcTx(dpinfo.BtcAddress, dpinfo.BtcTx)

		// bkt := tx.Bucket(depositInfoBkt)
		// if bkt == nil {
		// 	return bucketNotExistErr(depositInfoBkt)
		// }

		// // update index of skycoin address and the deposit seq
		// skyIndexBkt := tx.Bucket(skyDepositSeqsIndexBkt)
		// if skyIndexBkt == nil {
		// 	return bucketNotExistErr(skyDepositSeqsIndexBkt)
		// }

		// var addrs []string
		// addrsBytes := skyIndexBkt.Get([]byte(dpinfo.SkyAddress))
		// if addrsBytes != nil {
		// 	if err := json.Unmarshal(addrsBytes, &addrs); err != nil {
		// 		return err
		// 	}
		// }

		// // write deposit info back to db
		// v, err := json.Marshal(dpinfo)
		// if err != nil {
		// 	return err
		// }

		// if err := bkt.Put([]byte(dpinfo.BtcAddress), v); err != nil {
		// 	return err
		// }

		// // add btc address to bind list
		// addrs = append(addrs, dpinfo.BtcAddress)
		// v, err = json.Marshal(addrs)
		// if err != nil {
		// 	return err
		// }

		// if err := skyIndexBkt.Put([]byte(dpinfo.SkyAddress), v); err != nil {
		// 	return err
		// }

		// // bind address
		// bindAddrBkt := tx.Bucket(bindAddressBkt)
		// if bindAddrBkt == nil {
		// 	return bucketNotExistErr(bindAddressBkt)
		// }

		// if v := bindAddrBkt.Get([]byte(dpinfo.BtcAddress)); v != nil {
		// 	return fmt.Errorf("address %s already binded", dpinfo.BtcAddress)
		// }

		// if err := bindAddrBkt.Put([]byte(dpinfo.BtcAddress),
		// 	[]byte(dpinfo.SkyAddress)); err != nil {
		// 	return fmt.Errorf("Bind address failed: %v", err)
		// }

		// // update the caches
		// s.cache.setDepositInfo(dpinfo)
		// s.cache.addSkyIndex(dpinfo.SkyAddress, dpinfo.BtcAddress)
		// s.cache.setBindAddr(dpinfo.BtcAddress, dpinfo.SkyAddress)

		return nil
	})
}

// GetDepositInfo returns depsoit info of given btc address
func (s *store) GetDepositInfo(btctx string) (DepositInfo, bool) {
	return s.cache.getDepositInfo(btctx)
}

// GetAllDepositInfo returns all deposit info
func (s *store) GetDepositInfoArray(flt DepositFilter) []DepositInfo {
	return s.cache.getDepositInfoArray(flt)
}

// UpdateDepositStatus updates deposit info
func (s *store) UpdateDepositInfo(btctx string, updateFunc func(DepositInfo) DepositInfo) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(depositInfoBkt)
		if bkt == nil {
			return bucketNotExistErr(depositInfoBkt)
		}

		v := bkt.Get([]byte(btctx))
		if v == nil {
			return fmt.Errorf("DepositInfo of btc tx %s doesn't exist in db", btctx)
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

		if err := bkt.Put([]byte(btctx), d); err != nil {
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
		// get btc txids of btc address
		txs := s.cache.getBtcTxs(btcAddr)
		if len(txs) == 0 {
			dpis = append(dpis, DepositInfo{
				BtcAddress: btcAddr,
				SkyAddress: skyAddr,
				UpdatedAt:  time.Now().UTC().Unix(),
			})
			continue
		}

		for _, tx := range txs {
			dpi, ok := s.cache.getDepositInfo(tx)
			if !ok {
				return []DepositInfo{}, fmt.Errorf("Get deposit info of btc tx %s from cache failed", tx)
			}
			dpis = append(dpis, dpi)
		}
	}

	// sort the dpis by update time
	sort.Slice(dpis, func(i, j int) bool {
		return dpis[i].UpdatedAt < dpis[j].UpdatedAt
	})

	// renumber the seqs in the dpis
	for i := range dpis {
		dpis[i].Seq = uint64(i)
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
	btcTxs              map[string][]string
}

func loadCache(db *bolt.DB) (*cache, error) {
	c := cache{
		depositInfo:         make(map[string]DepositInfo),
		skyDepositSeqsIndex: make(map[string][]string),
		bindAddress:         make(map[string]string),
		btcTxs:              make(map[string][]string),
	}

	if err := db.View(func(tx *bolt.Tx) error {
		// init deposit info cache
		dpiBkt := tx.Bucket(depositInfoBkt)
		if dpiBkt == nil {
			return bucketNotExistErr(depositInfoBkt)
		}

		if err := dpiBkt.ForEach(func(k, v []byte) error {
			btctx := string(k)
			var dpi DepositInfo
			if err := json.Unmarshal(v, &dpi); err != nil {
				return err
			}

			c.depositInfo[btctx] = dpi
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

		if err := bindAddrBkt.ForEach(func(k, v []byte) error {
			btcAddr := string(k)
			skyAddr := string(v)
			c.bindAddress[btcAddr] = skyAddr
			return nil
		}); err != nil {
			return err
		}

		btBkt := tx.Bucket(btcTxsBkt)
		if btBkt == nil {
			return bucketNotExistErr(btcTxsBkt)
		}

		return btBkt.ForEach(func(k, v []byte) error {
			btcaddr := string(k)
			var txs []string
			if err := json.Unmarshal(v, &txs); err != nil {
				return err
			}

			c.btcTxs[btcaddr] = txs
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
	c.depositInfo[dpi.BtcTx] = dpi
	c.Unlock()
	log.Printf("setDepositInfo: %+v\n", dpi)
}

func (c *cache) getDepositInfo(btctx string) (DepositInfo, bool) {
	c.RLock()
	defer c.RUnlock()
	log.Println("depositInfo:", c.depositInfo)
	dpi, ok := c.depositInfo[btctx]
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

func (c *cache) addBtcTx(btcaddr, tx string) {
	c.Lock()
	txs, ok := c.btcTxs[btcaddr]
	if ok {
		c.btcTxs[btcaddr] = append(txs, tx)
	} else {
		c.btcTxs[btcaddr] = []string{tx}
	}
	c.Unlock()
}

func (c *cache) getBtcTxs(btcaddr string) []string {
	c.RLock()
	defer c.RUnlock()
	return c.btcTxs[btcaddr]
}

func uint64ToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	return b
}

func bytesToUint64(b []byte) uint64 {
	return binary.LittleEndian.Uint64(b)
}

func getBktValue(tx *bolt.Tx, bktName []byte, key []byte, value interface{}) (bool, error) {
	refV := reflect.ValueOf(value)
	if refV.Kind() != reflect.Ptr {
		return false, fmt.Errorf("value is not setable")
	}

	bkt := tx.Bucket(bktName)
	if bkt == nil {
		return false, bucketNotExistErr(bktName)
	}

	v := bkt.Get(key)
	if v == nil {
		return false, nil
	}

	if err := json.Unmarshal(v, value); err != nil {
		return false, fmt.Errorf("decode value failed: %v", err)
	}

	return true, nil
}

func putBktValue(tx *bolt.Tx, bktName []byte, key []byte, value interface{}) error {
	bkt := tx.Bucket(bktName)
	if bkt == nil {
		return bucketNotExistErr(bktName)
	}

	tp := reflect.TypeOf(value)
	switch tp.Kind() {
	case reflect.String:
		return bkt.Put(key, []byte(value.(string)))
	default:
		v, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode value failed: %v", err)
		}
		return bkt.Put(key, v)
	}
}
