package exchange

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
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

	// ErrNoBoundAddress is returned if no skycoin address is bound to a deposit's address
	ErrNoBoundAddress = errors.New("Deposit has no bound skycoin address")
)

// Storer interface for exchange storage
type Storer interface {
	GetBindAddress(string) (string, error)
	GetBindAddressTx(*bolt.Tx, string) (string, error)
	BindAddress(string, string) error
	GetOrCreateDepositInfo(scanner.Deposit, int64) (DepositInfo, error)
	AddDepositInfo(DepositInfo) error
	AddDepositInfoTx(*bolt.Tx, DepositInfo) error
	GetDepositInfo(string) (DepositInfo, error)
	GetDepositInfoTx(*bolt.Tx, string) (DepositInfo, error)
	GetDepositInfoArray(DepositFilter) ([]DepositInfo, error)
	GetDepositInfoOfSkyAddress(string) ([]DepositInfo, error)
	UpdateDepositInfo(string, func(DepositInfo) DepositInfo) (DepositInfo, error)
	GetSkyBindBtcAddresses(string) ([]string, error)
	GetSkyBindBtcAddressesTx(*bolt.Tx, string) ([]string, error)
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
		skyAddr, err = s.GetBindAddressTx(tx, btcAddr)
		return err
	})
	return skyAddr, err
}

// GetBindAddressTx returns bound skycoin address of given bitcoin address.
// If no skycoin address is found, returns empty string and nil error.
func (s *Store) GetBindAddressTx(tx *bolt.Tx, btcAddr string) (string, error) {
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
	return s.db.Update(func(tx *bolt.Tx) error {
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
		di, err := s.GetDepositInfoTx(tx, dv.TxN())

		switch err.(type) {
		case nil:
			finalDepositInfo = di
			return nil

		case dbutil.ObjectNotExistErr:
			log.Info("DepositInfo not found in DB, inserting")
			skyAddr, err := s.GetBindAddressTx(tx, dv.Address)
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
				SkyAddress: skyAddr,
				BtcAddress: dv.Address,
				BtcTx:      dv.TxN(),
				Status:     StatusWaitSend,
				// Save the rate at the time this deposit was noticed
				SkyBtcRate: rate,
				Deposit:    dv,
			}

			log = log.WithField("depositInfo", di)

			if err := s.AddDepositInfoTx(tx, di); err != nil {
				err = fmt.Errorf("AddDepositInfoTx failed: %v", err)
				log.WithError(err).Error()
				return err
			}

			finalDepositInfo = di

			return nil

		default:
			err = fmt.Errorf("GetDepositInfo failed: %v", err)
			log.WithError(err).Error()
			return err
		}
	}); err != nil {
		return DepositInfo{}, err
	}

	return finalDepositInfo, nil

}

func isValidBtcTx(btcTx string) bool {
	if btcTx == "" {
		return false
	}

	pts := strings.Split(btcTx, ":")
	if len(pts) != 2 {
		return false
	}

	if pts[0] == "" || pts[1] == "" {
		return false
	}

	_, err := strconv.ParseInt(pts[1], 10, 64)
	if err != nil {
		return false
	}

	return true
}

// AddDepositInfo adds deposit info into storage, return seq or error
func (s *Store) AddDepositInfo(di DepositInfo) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return s.AddDepositInfoTx(tx, di)
	})
}

// AddDepositInfoTx adds deposit info into storage, return seq or error
func (s *Store) AddDepositInfoTx(tx *bolt.Tx, di DepositInfo) error {
	log := s.log.WithField("depositInfo", di)

	if !isValidBtcTx(di.BtcTx) {
		log.Error("Invalid depositInfo.BtcTx")
		return fmt.Errorf("btc txid \"%s\" is empty/invalid", di.BtcTx)
	}

	if di.BtcAddress == "" {
		return errors.New("btc address is empty")
	}

	// check if the dpi with BtcTx already exist
	if hasKey, err := dbutil.BucketHasKey(tx, depositInfoBkt, di.BtcTx); err != nil {
		return err
	} else if hasKey {
		return fmt.Errorf("deposit info of btctx %s already exists", di.BtcTx)
	}

	seq, err := dbutil.NextSequence(tx, depositInfoBkt)
	if err != nil {
		return err
	}

	di.Seq = seq
	di.UpdatedAt = time.Now().UTC().Unix()

	if err := dbutil.PutBucketValue(tx, depositInfoBkt, di.BtcTx, di); err != nil {
		return err
	}

	// update btc_txids bucket
	var txs []string
	if err := dbutil.GetBucketObject(tx, btcTxsBkt, di.BtcAddress, &txs); err != nil {
		switch err.(type) {
		case dbutil.ObjectNotExistErr:
		default:
			return err
		}
	}

	txs = append(txs, di.BtcTx)
	return dbutil.PutBucketValue(tx, btcTxsBkt, di.BtcAddress, txs)
}

// GetDepositInfo returns depsoit info of given btc address
func (s *Store) GetDepositInfo(btcTx string) (DepositInfo, error) {
	var di DepositInfo

	err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		di, err = s.GetDepositInfoTx(tx, btcTx)
		return err
	})

	return di, err
}

// GetDepositInfoTx returns depsoit info of given btc address
func (s *Store) GetDepositInfoTx(tx *bolt.Tx, btcTx string) (DepositInfo, error) {
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
		btcAddrs, err := s.GetSkyBindBtcAddressesTx(tx, skyAddr)
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

		log = log.WithField("DepositInfo", dpi)

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
		addrs, err = s.GetSkyBindBtcAddressesTx(tx, skyAddr)
		return err
	}); err != nil {
		return nil, err
	}

	return addrs, nil
}

// GetSkyBindBtcAddressesTx returns the btc addresses of the given sky address bound
func (s *Store) GetSkyBindBtcAddressesTx(tx *bolt.Tx, skyAddr string) ([]string, error) {
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
