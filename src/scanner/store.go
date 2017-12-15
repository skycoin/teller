package scanner

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"

	"github.com/skycoin/teller/src/util/dbutil"
	"github.com/skycoin/teller/src/util/mathutil"
)

// CoinTypeBTC is BTC coin type
const CoinTypeBTC = "BTC"

// CoinTypeETH is ETH coin type
const CoinTypeETH = "ETH"

var (
	// scan meta info bucket
	scanMetaBktPrefix = []byte("scan_meta")

	// deposit value bucket
	depositBkt = []byte("deposit_value")

	// deposit address bucket
	depositAddressesKey = "deposit_addresses"

	// deposit values index list bucket
	dvIndexListKey = "dv_index_list"

	ErrUnsupportedCoinType = errors.New("unsupported coin type")
)

// DepositsEmptyErr is returned if there are no deposit values
type DepositsEmptyErr struct{}

func (e DepositsEmptyErr) Error() string {
	return "No deposit values available"
}

// DepositExistsErr is returned when a deposit value already exists
type DepositExistsErr struct{}

func (e DepositExistsErr) Error() string {
	return "Deposit value already exists"
}

// DuplicateDepositAddressErr is returned if a certain deposit address already
// exists when adding it to a bucket
type DuplicateDepositAddressErr struct {
	Address string
}

func (e DuplicateDepositAddressErr) Error() string {
	return fmt.Sprintf("Deposit address \"%s\" already exists", e.Address)
}

// NewDuplicateDepositAddressErr return a DuplicateDepositAddressErr
func NewDuplicateDepositAddressErr(addr string) error {
	return DuplicateDepositAddressErr{
		Address: addr,
	}
}

// Storer interface for scanner meta info storage
type Storer interface {
	GetScanAddresses(string) ([]string, error)
	AddScanAddress(string, string) error
	SetDepositProcessed(string) error
	GetUnprocessedDeposits() ([]Deposit, error)
	ScanBlock(interface{}, string) ([]Deposit, error)
}

// Store records scanner meta info for BTC deposits
type Store struct {
	db             *bolt.DB
	log            logrus.FieldLogger
	scanHandlerMap map[string]func(blockInfo interface{}, depositAddrs []string) ([]Deposit, error)
}

// NewStore creates a scanner Store
func NewStore(log logrus.FieldLogger, db *bolt.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("new Store failed: db is nil")
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(depositBkt)
		return err
	}); err != nil {
		return nil, err
	}
	return &Store{
		db:             db,
		log:            log,
		scanHandlerMap: make(map[string]func(blockInfo interface{}, depositAddrs []string) ([]Deposit, error)),
	}, nil
}

//AddSupportedCoin create scaninfo bucket and callback for specified coin
func (s *Store) AddSupportedCoin(coinType string, scanHandler func(blockInfo interface{}, depositAddrs []string) ([]Deposit, error)) error {
	if scanHandler == nil {
		return errors.New("Scan handler cann't nil")
	}
	_, exists := s.scanHandlerMap[coinType]
	if !exists {
		s.scanHandlerMap[coinType] = scanHandler
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		scanBktFullName := dbutil.ByteJoin(scanMetaBktPrefix, coinType, "_")
		if _, err := tx.CreateBucketIfNotExists(scanBktFullName); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

// GetScanAddresses returns all scan addresses
func (s *Store) GetScanAddresses(coinType string) ([]string, error) {
	var addrs []string

	if err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		addrs, err = s.getScanAddressesTx(tx, coinType)
		return err
	}); err != nil {
		return nil, err
	}

	return addrs, nil
}

// getScanAddressesTx returns all scan addresses in a bolt.Tx
func (s *Store) getScanAddressesTx(tx *bolt.Tx, coinType string) ([]string, error) {
	var addrs []string

	scanBktFullName := dbutil.ByteJoin(scanMetaBktPrefix, coinType, "_")
	if err := dbutil.GetBucketObject(tx, scanBktFullName, depositAddressesKey, &addrs); err != nil {
		switch err.(type) {
		case dbutil.ObjectNotExistErr:
			err = nil
		default:
			return nil, err
		}
	}

	if len(addrs) == 0 {
		addrs = nil
	}

	return addrs, nil
}

// AddScanAddress adds an address to the scan list
func (s *Store) AddScanAddress(addr, coinType string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		addrs, err := s.getScanAddressesTx(tx, coinType)
		if err != nil {
			return err
		}

		for _, a := range addrs {
			if a == addr {
				return NewDuplicateDepositAddressErr(addr)
			}
		}

		addrs = append(addrs, addr)

		scanBktFullName := dbutil.ByteJoin(scanMetaBktPrefix, coinType, "_")
		return dbutil.PutBucketValue(tx, scanBktFullName, depositAddressesKey, addrs)
	})
}

// SetDepositProcessed marks a Deposit as processed
func (s *Store) SetDepositProcessed(dvKey string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		var dv Deposit
		if err := dbutil.GetBucketObject(tx, depositBkt, dvKey, &dv); err != nil {
			return err
		}

		if dv.ID() != dvKey {
			return errors.New("CRITICAL ERROR: dv.ID() != dvKey")
		}

		dv.Processed = true

		return dbutil.PutBucketValue(tx, depositBkt, dv.ID(), dv)
	})
}

// GetUnprocessedDeposits returns all Deposits not marked as Processed
func (s *Store) GetUnprocessedDeposits() ([]Deposit, error) {
	var dvs []Deposit

	if err := s.db.View(func(tx *bolt.Tx) error {
		return dbutil.ForEach(tx, depositBkt, func(k, v []byte) error {
			var dv Deposit
			if err := json.Unmarshal(v, &dv); err != nil {
				return err
			}

			if !dv.Processed {
				dvs = append(dvs, dv)
			}

			return nil
		})
	}); err != nil {
		return nil, err
	}

	return dvs, nil
}

// pushDepositTx adds an Deposit in a bolt.Tx
// Returns DepositExistsErr if the deposit already exists
func (s *Store) pushDepositTx(tx *bolt.Tx, dv Deposit) error {
	key := dv.ID()

	// Check if the deposit value already exists
	if hasKey, err := dbutil.BucketHasKey(tx, depositBkt, key); err != nil {
		return err
	} else if hasKey {
		return DepositExistsErr{}
	}

	// Save deposit value
	return dbutil.PutBucketValue(tx, depositBkt, key, dv)
}

// ScanBlock scans a coin block for deposits and adds them
// If the deposit already exists, the result is omitted from the returned list
func (s *Store) ScanBlock(blockInfo interface{}, coinType string) ([]Deposit, error) {
	callback, exists := s.scanHandlerMap[coinType]
	if !exists {
		var dvs []Deposit
		return dvs, ErrUnsupportedCoinType
	}
	return s.scanBlock(blockInfo, coinType, callback)
}

// scanBlock scans a coin block for deposits and adds them
// 1. get deposit address by coinType
// 2. call callback function to get deposit
// 3. push deposit into db, finished at one transaction
func (s *Store) scanBlock(blockInfo interface{}, coinType string, scanBlockCallback func(blockInfo interface{}, depositAddrs []string) ([]Deposit, error)) ([]Deposit, error) {
	var dvs []Deposit

	if err := s.db.Update(func(tx *bolt.Tx) error {
		addrs, err := s.getScanAddressesTx(tx, coinType)
		if err != nil {
			s.log.WithError(err).Error("getScanAddressesTx failed")
			return err
		}

		deposits, err := scanBlockCallback(blockInfo, addrs)
		if err != nil {
			s.log.WithError(err).Error("ScanBlock failed")
			return err
		}

		for _, dv := range deposits {
			if err := s.pushDepositTx(tx, dv); err != nil {
				log := s.log.WithField("deposit", dv)
				switch err.(type) {
				case DepositExistsErr:
					log.Warning("Deposit already exists in db")
					continue
				default:
					log.WithError(err).Error("pushDepositTx failed")
					return err
				}
			}

			dvs = append(dvs, dv)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return dvs, nil
}

// ScanBTCBlock scan the given block and returns the next block hash or error
func ScanBTCBlock(blockInfo interface{}, depositAddrs []string) ([]Deposit, error) {
	var dv []Deposit
	block, ok := blockInfo.(*btcjson.GetBlockVerboseResult)
	if !ok {
		return dv, errors.New("convert to GetBlockVerboseResult failed")
	}
	if len(block.RawTx) == 0 {
		return nil, ErrBtcdTxindexDisabled
	}

	addrMap := map[string]struct{}{}
	for _, a := range depositAddrs {
		addrMap[a] = struct{}{}
	}

	for _, tx := range block.RawTx {
		for _, v := range tx.Vout {
			amt, err := btcutil.NewAmount(v.Value)
			if err != nil {
				return nil, err
			}

			for _, a := range v.ScriptPubKey.Addresses {
				if _, ok := addrMap[a]; ok {
					dv = append(dv, Deposit{
						CoinType: CoinTypeBTC,
						Address:  a,
						Value:    int64(amt),
						Height:   block.Height,
						Tx:       tx.Txid,
						N:        v.N,
					})
				}
			}
		}
	}

	return dv, nil
}

// ScanETHBlock scan the given block and returns the next block hash or error
func ScanETHBlock(blockInfo interface{}, depositAddrs []string) ([]Deposit, error) {
	var dv []Deposit
	block, ok := blockInfo.(*types.Block)
	if !ok {
		return dv, errors.New("convert to types.Block failed")
	}
	addrMap := map[string]struct{}{}
	for _, a := range depositAddrs {
		addrMap[a] = struct{}{}
	}

	for i, tx := range block.Transactions() {
		to := tx.To()
		if to == nil {
			//this is a contract transcation
			continue
		}
		//1 eth = 1e18 wei ,tx.Value() is very big that may overflow(int64), so store it as Gwei(1Gwei=1e9wei) and recover it when used
		amt := mathutil.Wei2Gwei(tx.Value())
		a := strings.ToLower(to.String())
		if _, ok := addrMap[a]; ok {
			dv = append(dv, Deposit{
				CoinType: CoinTypeETH,
				Address:  a,
				Value:    amt,
				Height:   int64(block.NumberU64()),
				Tx:       tx.Hash().String(),
				N:        uint32(i),
			})
		}
	}

	return dv, nil
}
