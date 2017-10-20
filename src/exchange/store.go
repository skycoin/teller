package exchange

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"

	"github.com/skycoin/teller/src/scanner"
	"github.com/skycoin/teller/src/util/dbutil"
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

	// ErrAddressAlreadyBound is returned if an address has already been bound to a SKY address
	ErrAddressAlreadyBound = errors.New("Address already bound to a SKY address")
)

// Storer interface for exchange storage
type Storer interface {
	GetBindAddress(btcAddr string) (string, error)
	BindAddress(skyAddr, btcAddr string) error
	GetOrCreateDepositInfo(scanner.Deposit, int64) (DepositInfo, error)
	GetDepositInfoArray(DepositFilter) ([]DepositInfo, error)
	GetDepositInfoOfSkyAddress(string) ([]DepositInfo, error)
	UpdateDepositInfo(string, func(DepositInfo) DepositInfo) (DepositInfo, error)
	GetSkyBindBtcAddresses(string) ([]string, error)
}

// Store storage for exchange
type Store struct {
	db  *bolt.DB
	log logrus.FieldLogger
}

// NewStore creates a Store instance
func NewStore(log logrus.FieldLogger, db *bolt.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("new exchange Store failed, db is nil")
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		// create exchange meta bucket if not exist
		if _, err := tx.CreateBucketIfNotExists(exchangeMetaBkt); err != nil {
			return dbutil.NewCreateBucketFailedErr(exchangeMetaBkt, err)
		}

		// create deposit status bucket if not exist
		if _, err := tx.CreateBucketIfNotExists(depositInfoBkt); err != nil {
			return dbutil.NewCreateBucketFailedErr(depositInfoBkt, err)
		}

		// create bind address bucket if not exist
		if _, err := tx.CreateBucketIfNotExists(bindAddressBkt); err != nil {
			return dbutil.NewCreateBucketFailedErr(bindAddressBkt, err)
		}

		if _, err := tx.CreateBucketIfNotExists(skyDepositSeqsIndexBkt); err != nil {
			return dbutil.NewCreateBucketFailedErr(skyDepositSeqsIndexBkt, err)
		}

		if _, err := tx.CreateBucketIfNotExists(btcTxsBkt); err != nil {
			return dbutil.NewCreateBucketFailedErr(btcTxsBkt, err)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &Store{
		db:  db,
		log: log.WithField("prefix", "exchange.Store"),
	}, nil

}

// GetBindAddress returns bound skycoin address of given bitcoin address.
// If no skycoin address is found, returns empty string and nil error.
func (s *Store) GetBindAddress(btcAddr string) (string, error) {
	var skyAddr string
	err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		skyAddr, err = s.getBindAddressTx(tx, btcAddr)
		return err
	})
	return skyAddr, err
}

// getBindAddressTx returns bound skycoin address of given bitcoin address.
// If no skycoin address is found, returns empty string and nil error.
func (s *Store) getBindAddressTx(tx *bolt.Tx, btcAddr string) (string, error) {
	skyAddr, err := dbutil.GetBucketString(tx, bindAddressBkt, btcAddr)

	switch err.(type) {
	case nil:
		return skyAddr, nil
	case dbutil.ObjectNotExistErr:
		return "", nil
	default:
		return "", err
	}
}

// BindAddress binds a skycoin address to a BTC address
func (s *Store) BindAddress(skyAddr, btcAddr string) error {
	log := s.log.WithField("skyAddr", skyAddr)
	log = log.WithField("btcAddr", btcAddr)
	return s.db.Update(func(tx *bolt.Tx) error {
		existingSkyAddr, err := s.getBindAddressTx(tx, btcAddr)
		if err != nil {
			return err
		}

		if existingSkyAddr != "" {
			err := ErrAddressAlreadyBound
			log.WithError(err).Error("Attempted to bind an address twice")
			return err
		}

		// update index of skycoin address and the deposit seq
		var addrs []string
		if err := dbutil.GetBucketObject(tx, skyDepositSeqsIndexBkt, skyAddr, &addrs); err != nil {
			switch err.(type) {
			case dbutil.ObjectNotExistErr:
			default:
				return err
			}
		}

		addrs = append(addrs, btcAddr)
		if err := dbutil.PutBucketValue(tx, skyDepositSeqsIndexBkt, skyAddr, addrs); err != nil {
			return err
		}

		return dbutil.PutBucketValue(tx, bindAddressBkt, btcAddr, skyAddr)
	})
}

// GetOrCreateDepositInfo creates a DepositInfo unless one exists with the DepositInfo.BtcTx key,
// in which case it returns the existing DepositInfo.
func (s *Store) GetOrCreateDepositInfo(dv scanner.Deposit, rate int64) (DepositInfo, error) {
	log := s.log.WithField("deposit", dv)
	log = log.WithField("rate", rate)

	var finalDepositInfo DepositInfo
	if err := s.db.Update(func(tx *bolt.Tx) error {
		di, err := s.getDepositInfoTx(tx, dv.TxN())

		switch err.(type) {
		case nil:
			finalDepositInfo = di
			return nil

		case dbutil.ObjectNotExistErr:
			log.Info("DepositInfo not found in DB, inserting")
			skyAddr, err := s.getBindAddressTx(tx, dv.Address)
			if err != nil {
				err = fmt.Errorf("GetBindAddress failed: %v", err)
				log.WithError(err).Error()
				return err
			}

			if skyAddr == "" {
				err = ErrNoBoundAddress
				log.WithError(err).Error()
				return err
			}

			log = log.WithField("skyAddr", skyAddr)

			di := DepositInfo{
				SkyAddress:   skyAddr,
				BtcAddress:   dv.Address,
				BtcTx:        dv.TxN(),
				Status:       StatusWaitSend,
				DepositValue: dv.Value,
				// Save the rate at the time this deposit was noticed
				SkyBtcRate: rate,
				Deposit:    dv,
			}

			log = log.WithField("depositInfo", di)

			updatedDi, err := s.addDepositInfoTx(tx, di)
			if err != nil {
				err = fmt.Errorf("addDepositInfoTx failed: %v", err)
				log.WithError(err).Error()
				return err
			}

			finalDepositInfo = updatedDi

			return nil

		default:
			err = fmt.Errorf("getDepositInfo failed: %v", err)
			log.WithError(err).Error()
			return err
		}
	}); err != nil {
		return DepositInfo{}, err
	}

	return finalDepositInfo, nil

}

// addDepositInfo adds deposit info into storage, return seq or error
func (s *Store) addDepositInfo(di DepositInfo) (DepositInfo, error) {
	var updatedDi DepositInfo
	if err := s.db.Update(func(tx *bolt.Tx) error {
		var err error
		updatedDi, err = s.addDepositInfoTx(tx, di)
		return err
	}); err != nil {
		return di, err
	}

	return updatedDi, nil
}

// addDepositInfoTx adds deposit info into storage, return seq or error
func (s *Store) addDepositInfoTx(tx *bolt.Tx, di DepositInfo) (DepositInfo, error) {
	log := s.log.WithField("depositInfo", di)

	// check if the dpi with BtcTx already exist
	if hasKey, err := dbutil.BucketHasKey(tx, depositInfoBkt, di.BtcTx); err != nil {
		return di, err
	} else if hasKey {
		return di, fmt.Errorf("deposit info of btctx \"%s\" already exists", di.BtcTx)
	}

	seq, err := dbutil.NextSequence(tx, depositInfoBkt)
	if err != nil {
		return di, err
	}

	updatedDi := di
	updatedDi.Seq = seq
	log.Println("SEQUENCE", seq)
	updatedDi.UpdatedAt = time.Now().UTC().Unix()

	if err := updatedDi.ValidateForStatus(); err != nil {
		log.WithError(err).Error("FIXME: Constructed invalid DepositInfo")
		return di, err
	}

	if err := dbutil.PutBucketValue(tx, depositInfoBkt, updatedDi.BtcTx, updatedDi); err != nil {
		return di, err
	}

	// update btc_txids bucket
	var txs []string
	if err := dbutil.GetBucketObject(tx, btcTxsBkt, updatedDi.BtcAddress, &txs); err != nil {
		switch err.(type) {
		case dbutil.ObjectNotExistErr:
		default:
			return di, err
		}
	}

	txs = append(txs, updatedDi.BtcTx)
	if err := dbutil.PutBucketValue(tx, btcTxsBkt, updatedDi.BtcAddress, txs); err != nil {
		return di, err
	}

	return updatedDi, nil
}

// getDepositInfo returns depsoit info of given btc address
func (s *Store) getDepositInfo(btcTx string) (DepositInfo, error) {
	var di DepositInfo

	err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		di, err = s.getDepositInfoTx(tx, btcTx)
		return err
	})

	return di, err
}

// getDepositInfoTx returns depsoit info of given btc address
func (s *Store) getDepositInfoTx(tx *bolt.Tx, btcTx string) (DepositInfo, error) {
	var dpi DepositInfo

	if err := dbutil.GetBucketObject(tx, depositInfoBkt, btcTx, &dpi); err != nil {
		return DepositInfo{}, err
	}

	return dpi, nil
}

// GetDepositInfoArray returns filtered deposit info
func (s *Store) GetDepositInfoArray(flt DepositFilter) ([]DepositInfo, error) {
	var dpis []DepositInfo

	if err := s.db.View(func(tx *bolt.Tx) error {
		return dbutil.ForEach(tx, depositInfoBkt, func(k, v []byte) error {
			var dpi DepositInfo
			if err := json.Unmarshal(v, &dpi); err != nil {
				return err
			}

			if flt(dpi) {
				dpis = append(dpis, dpi)
			}

			return nil
		})
	}); err != nil {
		return nil, err
	}

	return dpis, nil
}

// GetDepositInfoOfSkyAddress returns all deposit info that are bound
// to the given skycoin address
func (s *Store) GetDepositInfoOfSkyAddress(skyAddr string) ([]DepositInfo, error) {
	var dpis []DepositInfo

	if err := s.db.View(func(tx *bolt.Tx) error {
		// TODO: DB queries in a loop, may need restructuring for performance
		btcAddrs, err := s.getSkyBindBtcAddressesTx(tx, skyAddr)
		if err != nil {
			return err
		}

		for _, btcAddr := range btcAddrs {
			var txns []string
			if err := dbutil.GetBucketObject(tx, btcTxsBkt, btcAddr, &txns); err != nil {
				switch err.(type) {
				case dbutil.ObjectNotExistErr:
				default:
					return err
				}
			}

			// If this db has no DepositInfo records yet, it means the scanner
			// has not sent a deposit to the exchange, so the status is
			// StatusWaitDeposit.
			if len(txns) == 0 {
				dpis = append(dpis, DepositInfo{
					Status:     StatusWaitDeposit,
					BtcAddress: btcAddr,
					SkyAddress: skyAddr,
					UpdatedAt:  time.Now().UTC().Unix(),
				})
			}

			for _, txn := range txns {
				var dpi DepositInfo
				if err := dbutil.GetBucketObject(tx, depositInfoBkt, txn, &dpi); err != nil {
					return err
				}

				dpis = append(dpis, dpi)
			}
		}

		return nil
	}); err != nil {
		return nil, err
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

// UpdateDepositInfo updates deposit info. The update func takes a DepositInfo
// and returns a modified copy of it.
func (s *Store) UpdateDepositInfo(btcTx string, update func(DepositInfo) DepositInfo) (DepositInfo, error) {
	log := s.log.WithField("btcTx", btcTx)

	var dpi DepositInfo
	if err := s.db.Update(func(tx *bolt.Tx) error {
		if err := dbutil.GetBucketObject(tx, depositInfoBkt, btcTx, &dpi); err != nil {
			return err
		}

		log = log.WithField("depositInfo", dpi)

		if dpi.BtcTx != btcTx {
			log.Error("DepositInfo.BtcTx does not match btcTx")
			err := fmt.Errorf("DepositInfo %+v saved under different key %s", dpi, btcTx)
			return err
		}

		dpi = update(dpi)
		dpi.UpdatedAt = time.Now().UTC().Unix()

		return dbutil.PutBucketValue(tx, depositInfoBkt, btcTx, dpi)
	}); err != nil {
		return DepositInfo{}, err
	}

	return dpi, nil
}

// GetSkyBindBtcAddresses returns the btc addresses of the given sky address bound
func (s *Store) GetSkyBindBtcAddresses(skyAddr string) ([]string, error) {
	var addrs []string

	if err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		addrs, err = s.getSkyBindBtcAddressesTx(tx, skyAddr)
		return err
	}); err != nil {
		return nil, err
	}

	return addrs, nil
}

// getSkyBindBtcAddressesTx returns the btc addresses of the given sky address bound
func (s *Store) getSkyBindBtcAddressesTx(tx *bolt.Tx, skyAddr string) ([]string, error) {
	var addrs []string
	if err := dbutil.GetBucketObject(tx, skyDepositSeqsIndexBkt, skyAddr, &addrs); err != nil {
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
