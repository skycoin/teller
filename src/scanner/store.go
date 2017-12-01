package scanner

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/boltdb/bolt"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcutil"
	"github.com/sirupsen/logrus"

	"github.com/skycoin/teller/src/util/dbutil"
)

// CoinTypeBTC is BTC coin type
const CoinTypeBTC = "BTC"

var (
	// scan meta info bucket
	scanMetaBkt = []byte("scan_meta")

	// deposit value bucket
	depositBkt = []byte("deposit_value")

	// deposit address bucket
	depositAddressesKey = "deposit_addresses"

	// deposit values index list bucket
	dvIndexListKey = "dv_index_list"
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
	GetScanAddresses() ([]string, error)
	AddScanAddress(string) error
	SetDepositProcessed(string) error
	GetUnprocessedDeposits() ([]Deposit, error)
	ScanBlock(*btcjson.GetBlockVerboseResult) ([]Deposit, error)
}

// BTCStore records scanner meta info for BTC deposits
type BTCStore struct {
	db  *bolt.DB
	log logrus.FieldLogger
}

// NewStore creates a scanner BTCStore
func NewStore(log logrus.FieldLogger, db *bolt.DB) (*BTCStore, error) {
	if db == nil {
		return nil, errors.New("new BTCStore failed: db is nil")
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(scanMetaBkt); err != nil {
			return err
		}

		_, err := tx.CreateBucketIfNotExists(depositBkt)
		return err
	}); err != nil {
		return nil, err
	}

	return &BTCStore{
		db:  db,
		log: log,
	}, nil
}

// GetScanAddresses returns all scan addresses
func (s *BTCStore) GetScanAddresses() ([]string, error) {
	var addrs []string

	if err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		addrs, err = s.getScanAddressesTx(tx)
		return err
	}); err != nil {
		return nil, err
	}

	return addrs, nil
}

// getScanAddressesTx returns all scan addresses in a bolt.Tx
func (s *BTCStore) getScanAddressesTx(tx *bolt.Tx) ([]string, error) {
	var addrs []string

	if err := dbutil.GetBucketObject(tx, scanMetaBkt, depositAddressesKey, &addrs); err != nil {
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
func (s *BTCStore) AddScanAddress(addr string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		addrs, err := s.getScanAddressesTx(tx)
		if err != nil {
			return err
		}

		for _, a := range addrs {
			if a == addr {
				return NewDuplicateDepositAddressErr(addr)
			}
		}

		addrs = append(addrs, addr)

		return dbutil.PutBucketValue(tx, scanMetaBkt, depositAddressesKey, addrs)
	})
}

// SetDepositProcessed marks a Deposit as processed
func (s *BTCStore) SetDepositProcessed(dvKey string) error {
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
func (s *BTCStore) GetUnprocessedDeposits() ([]Deposit, error) {
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
func (s *BTCStore) pushDepositTx(tx *bolt.Tx, dv Deposit) error {
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

// ScanBlock scans a btc block for deposits and adds them
// If the deposit already exists, the result is omitted from the returned list
func (s *BTCStore) ScanBlock(block *btcjson.GetBlockVerboseResult) ([]Deposit, error) {
	var dvs []Deposit

	if err := s.db.Update(func(tx *bolt.Tx) error {
		addrs, err := s.getScanAddressesTx(tx)
		if err != nil {
			s.log.WithError(err).Error("getScanAddressesTx failed")
			return err
		}

		deposits, err := ScanBTCBlock(block, addrs)
		if err != nil {
			s.log.WithError(err).Error("ScanBTCBlock failed")
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
func ScanBTCBlock(block *btcjson.GetBlockVerboseResult, depositAddrs []string) ([]Deposit, error) {
	// Assert that RawTx matches Tx
	if len(block.RawTx) != len(block.Tx) {
		return nil, ErrBtcdTxindexDisabled
	}

	addrMap := map[string]struct{}{}
	for _, a := range depositAddrs {
		addrMap[a] = struct{}{}
	}

	var dv []Deposit
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
