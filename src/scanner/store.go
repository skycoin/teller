package scanner

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/boltdb/bolt"
	"github.com/skycoin/teller/src/util/dbutil"
)

var (
	scanMetaBkt = []byte("scan_meta")
	depositBkt  = []byte("deposit_value")

	lastScanBlockKey    = "last_scan_block"
	depositAddressesKey = "deposit_addresses"
	dvIndexListKey      = "dv_index_list" // deposit value index list

)

// DepositValuesEmptyErr is returned if there are no deposit values
type DepositValuesEmptyErr struct{}

func (e DepositValuesEmptyErr) Error() string {
	return "No deposit values available"
}

// DepositValueExistsErr is returned when a deposit value already exists
type DepositValueExistsErr struct{}

func (e DepositValueExistsErr) Error() string {
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

// store records scanner meta info
type store struct {
	db *bolt.DB
}

func newStore(db *bolt.DB) (*store, error) {
	if db == nil {
		return nil, errors.New("new store failed: db is nil")
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		// create LastScanBlock bucket if not exist
		if _, err := tx.CreateBucketIfNotExists(scanMetaBkt); err != nil {
			return err
		}

		_, err := tx.CreateBucketIfNotExists(depositBkt)
		return err
	}); err != nil {
		return nil, err
	}

	return &store{
		db: db,
	}, nil
}

// LastScanBlock stores the last scanned block's hash and height
type LastScanBlock struct {
	Hash   string
	Height int64
}

// getLastScanBlock returns the last scanned block hash and height
func (s *store) getLastScanBlock() (LastScanBlock, error) {
	var lsb LastScanBlock

	if err := s.db.View(func(tx *bolt.Tx) error {
		return dbutil.GetBucketObject(tx, scanMetaBkt, lastScanBlockKey, &lsb)
	}); err != nil {
		switch err.(type) {
		case dbutil.ObjectNotExistErr:
			err = nil
		default:
			return LastScanBlock{}, err
		}
	}

	return lsb, nil
}

func (s *store) setLastScanBlock(lsb LastScanBlock) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return dbutil.PutBucketValue(tx, scanMetaBkt, lastScanBlockKey, lsb)
	})
}

func (s *store) setLastScanBlockTx(tx *bolt.Tx, lsb LastScanBlock) error {
	return dbutil.PutBucketValue(tx, scanMetaBkt, lastScanBlockKey, lsb)
}

func (s *store) getScanAddressesTx(tx *bolt.Tx) ([]string, error) {
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

func (s *store) getScanAddresses() ([]string, error) {
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

func (s *store) addScanAddress(addr string) error {
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

func (s *store) removeScanAddr(addr string) error {
	// FIXME: This will be very slow with large number of scan addresses.
	// FIXME: Save scan addresses differently

	return s.db.Update(func(tx *bolt.Tx) error {
		addrs, err := s.getScanAddressesTx(tx)
		if err != nil {
			return err
		}

		idx := -1

		for i, a := range addrs {
			if a == addr {
				idx = i
				break
			}
		}

		if idx == -1 {
			return nil
		}

		addrs = append(addrs[:idx], addrs[idx+1:]...)
		return dbutil.PutBucketValue(tx, scanMetaBkt, depositAddressesKey, addrs)
	})
}

func (s *store) pushDepositValueTx(tx *bolt.Tx, dv Deposit) error {
	key := dv.TxN()

	// Check if the deposit value already exists
	if hasKey, err := dbutil.BucketHasKey(tx, depositBkt, key); err != nil {
		return err
	} else if hasKey {
		return DepositValueExistsErr{}
	}

	// Save deposit value
	return dbutil.PutBucketValue(tx, depositBkt, key, dv)
}

func (s *store) setDepositValueProcessed(dvKey string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		var dv Deposit
		if err := dbutil.GetBucketObject(tx, depositBkt, dvKey, &dv); err != nil {
			return err
		}

		if dv.TxN() != dvKey {
			return errors.New("CRITICAL ERROR: dv.Txn() != dvKey")
		}

		dv.Processed = true

		return dbutil.PutBucketValue(tx, depositBkt, dv.TxN(), dv)
	})
}

func (s *store) getUnprocessedDepositValues() ([]Deposit, error) {
	var dvs []Deposit

	if err := s.db.View(func(tx *bolt.Tx) error {
		return dbutil.ForEach(tx, depositBkt, func(k, v []byte) error {
			var dv Deposit
			if err := json.Unmarshal(v, &dvs); err != nil {
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
