package scanner

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"

	"github.com/skycoin/teller/src/util/dbutil"
)

const (
	// CoinTypeBTC is BTC coin type
	CoinTypeBTC = "BTC"
	// CoinTypeETH is ETH coin type
	CoinTypeETH = "ETH"
)

var (
	// DepositBkt maps a BTC transaction to a Deposit
	DepositBkt = []byte("deposit_value")

	// deposit address bucket
	depositAddressesKey = "deposit_addresses"

	// ErrUnsupportedCoinType unsupported coin type
	ErrUnsupportedCoinType = errors.New("unsupported coin type")
)

const scanMetaBktPrefix = "scan_meta"

// GetScanMetaBkt return the name of the scan_meta bucket for a given coin type
func GetScanMetaBkt(coinType string) ([]byte, error) {
	var suffix string
	switch coinType {
	case CoinTypeBTC:
		suffix = "btc"
	case CoinTypeETH:
		suffix = "eth"
	default:
		return nil, ErrUnsupportedCoinType
	}

	bktName := fmt.Sprintf("%s_%s", scanMetaBktPrefix, suffix)

	return []byte(bktName), nil
}

// MustGetScanMetaBkt panics if GetScanMetaBkt returns an error
func MustGetScanMetaBkt(coinType string) []byte {
	name, err := GetScanMetaBkt(coinType)
	if err != nil {
		panic(err)
	}
	return name
}

func init() {
	// Check that GetScanMetaBkt handles all possible coin types
	// TODO -- do similar init checks for other switches over coinType
	for _, ct := range GetCoinTypes() {
		name := MustGetScanMetaBkt(ct)
		if len(name) == 0 {
			panic(fmt.Sprintf("GetScanMetaBkt(%s) returned empty", ct))
		}
	}
}

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
	ScanBlock(*CommonBlock, string) ([]Deposit, error)
}

// Store records scanner meta info for BTC deposits
type Store struct {
	db  *bolt.DB
	log logrus.FieldLogger
}

// NewStore creates a scanner Store
func NewStore(log logrus.FieldLogger, db *bolt.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("new Store failed: db is nil")
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(DepositBkt)
		return err
	}); err != nil {
		return nil, err
	}
	return &Store{
		db:  db,
		log: log,
	}, nil
}

//AddSupportedCoin create scaninfo bucket and callback for specified coin
func (s *Store) AddSupportedCoin(coinType string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		scanBktFullName, err := GetScanMetaBkt(coinType)
		if err != nil {
			return err
		}

		_, err = tx.CreateBucketIfNotExists(scanBktFullName)
		return err
	})
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

	scanBktFullName, err := GetScanMetaBkt(coinType)
	if err != nil {
		return nil, err
	}

	if err := dbutil.GetBucketObject(tx, scanBktFullName, depositAddressesKey, &addrs); err != nil {
		switch err.(type) {
		case dbutil.ObjectNotExistErr:
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

		scanBktFullName, err := GetScanMetaBkt(coinType)
		if err != nil {
			return err
		}

		return dbutil.PutBucketValue(tx, scanBktFullName, depositAddressesKey, addrs)
	})
}

// SetDepositProcessed marks a Deposit as processed
func (s *Store) SetDepositProcessed(dvKey string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		var dv Deposit
		if err := dbutil.GetBucketObject(tx, DepositBkt, dvKey, &dv); err != nil {
			return err
		}

		if dv.ID() != dvKey {
			return errors.New("CRITICAL ERROR: dv.ID() != dvKey")
		}

		dv.Processed = true

		return dbutil.PutBucketValue(tx, DepositBkt, dv.ID(), dv)
	})
}

// GetUnprocessedDeposits returns all Deposits not marked as Processed
func (s *Store) GetUnprocessedDeposits() ([]Deposit, error) {
	var dvs []Deposit

	if err := s.db.View(func(tx *bolt.Tx) error {
		return dbutil.ForEach(tx, DepositBkt, func(k, v []byte) error {
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
	if hasKey, err := dbutil.BucketHasKey(tx, DepositBkt, key); err != nil {
		return err
	} else if hasKey {
		return DepositExistsErr{}
	}

	// Save deposit value
	return dbutil.PutBucketValue(tx, DepositBkt, key, dv)
}

// ScanBlock scans a coin block for deposits and adds them
// If the deposit already exists, the result is omitted from the returned list
func (s *Store) ScanBlock(block *CommonBlock, coinType string) ([]Deposit, error) {
	return s.scanBlock(block, coinType)
}

// scanBlock scans a coin block for deposits and adds them
// 1. get deposit address by coinType
// 2. call callback function to get deposit
// 3. push deposit into db, finished at one transaction
func (s *Store) scanBlock(block *CommonBlock, coinType string) ([]Deposit, error) {
	var dvs []Deposit

	if err := s.db.Update(func(tx *bolt.Tx) error {
		addrs, err := s.getScanAddressesTx(tx, coinType)
		if err != nil {
			s.log.WithError(err).Error("getScanAddressesTx failed")
			return err
		}

		deposits, err := scanSpecifiedBlock(block, coinType, addrs)
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
func scanSpecifiedBlock(block *CommonBlock, coinType string, depositAddrs []string) ([]Deposit, error) {
	var dv []Deposit

	addrMap := map[string]struct{}{}
	for _, a := range depositAddrs {
		addrMap[a] = struct{}{}
	}

	for _, tx := range block.RawTx {
		for _, v := range tx.Vout {
			amt := v.Value

			for _, a := range v.Addresses {
				if _, ok := addrMap[a]; ok {
					dv = append(dv, Deposit{
						CoinType: coinType,
						Address:  a,
						Value:    amt,
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
