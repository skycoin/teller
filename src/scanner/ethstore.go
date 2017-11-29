package scanner

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
	"github.com/skycoin/teller/src/util/dbutil"
)

var (
	// scan meta info bucket
	ethScanMetaBkt = []byte("scan_meta_eth")
)

// ETHStore records scanner meta info for ETH deposits
type ETHStore struct {
	db  *bolt.DB
	log logrus.FieldLogger
}

// NewStore creates a scanner ETHStore
func NewEthStore(log logrus.FieldLogger, db *bolt.DB) (*ETHStore, error) {
	if db == nil {
		return nil, errors.New("new ETHStore failed: db is nil")
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(ethScanMetaBkt); err != nil {
			return err
		}

		_, err := tx.CreateBucketIfNotExists(depositBkt)
		return err
	}); err != nil {
		return nil, err
	}

	return &ETHStore{
		db:  db,
		log: log,
	}, nil
}

// GetScanAddresses returns all scan addresses
func (s *ETHStore) GetScanAddresses() ([]string, error) {
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
func (s *ETHStore) getScanAddressesTx(tx *bolt.Tx) ([]string, error) {
	var addrs []string

	if err := dbutil.GetBucketObject(tx, ethScanMetaBkt, depositAddressesKey, &addrs); err != nil {
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
func (s *ETHStore) AddScanAddress(addr string) error {
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

		return dbutil.PutBucketValue(tx, ethScanMetaBkt, depositAddressesKey, addrs)
	})
}

// SetDepositProcessed marks a Deposit as processed
func (s *ETHStore) SetDepositProcessed(dvKey string) error {
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
func (s *ETHStore) GetUnprocessedDeposits() ([]Deposit, error) {
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
func (s *ETHStore) pushDepositTx(tx *bolt.Tx, dv Deposit) error {
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

// ScanBlock scans a eth block for deposits and adds them
// If the deposit already exists, the result is omitted from the returned list
func (s *ETHStore) ScanBlock(blockInfo interface{}) ([]Deposit, error) {
	var dvs []Deposit
	block, ok := blockInfo.(*types.Block)
	if !ok {
		s.log.Error("convert to types.Block failed")
		return dvs, errors.New("convert to types.Block failed")
	}

	if err := s.db.Update(func(tx *bolt.Tx) error {
		addrs, err := s.getScanAddressesTx(tx)
		if err != nil {
			s.log.WithError(err).Error("getScanAddressesTx failed")
			return err
		}

		deposits, err := ScanETHBlock(block, addrs)
		if err != nil {
			s.log.WithError(err).Error("ScanETHBlock failed")
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

// ScanETHBlock scan the given block and returns the next block hash or error
func ScanETHBlock(block *types.Block, depositAddrs []string) ([]Deposit, error) {
	addrMap := map[string]struct{}{}
	for _, a := range depositAddrs {
		addrMap[a] = struct{}{}
	}

	var dv []Deposit
	for i, tx := range block.Transactions() {
		//tx, err := s.GetTransaction(txid.Hash())
		//if err != nil {
		//log.WithError(err).Error("get eth transcation %s failed", txid.Hash().String())
		//continue
		//}
		to := tx.To()
		if to == nil {
			//this is a contract transcation
			continue
		}
		amt := tx.Value().Int64()
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
