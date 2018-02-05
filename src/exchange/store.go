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
	// ExchangeMetaBkt stores metadata about the exchange (unused)
	ExchangeMetaBkt = []byte("exchange_meta")

	// DepositInfoBkt maps a BTC transaction to a DepositInfo
	DepositInfoBkt = []byte("deposit_info")

	// BtcTxsBkt maps a BTC address to multiple BTC transactions
	BtcTxsBkt = []byte("btc_txs")

	// SkyDepositSeqsIndexBkt maps a SKY address to its BTC addresses
	SkyDepositSeqsIndexBkt = []byte("sky_deposit_seqs_index")

	// ErrAddressAlreadyBound is returned if an address has already been bound to a SKY address
	ErrAddressAlreadyBound = errors.New("Address already bound to a SKY address")
)

const bindAddressBktPrefix = "bind_address"

// GetBindAddressBkt returns the bind_address bucket name for a given coin type
func GetBindAddressBkt(coinType string) ([]byte, error) {
	var suffix string
	switch coinType {
	case scanner.CoinTypeBTC:
		suffix = "btc"
	case scanner.CoinTypeETH:
		suffix = "eth"
	default:
		return nil, scanner.ErrUnsupportedCoinType
	}

	bktName := fmt.Sprintf("%s_%s", bindAddressBktPrefix, suffix)

	return []byte(bktName), nil
}

// MustGetBindAddressBkt panics if GetBindAddressBkt returns an error
func MustGetBindAddressBkt(coinType string) []byte {
	name, err := GetBindAddressBkt(coinType)
	if err != nil {
		panic(err)
	}
	return name
}

func init() {
	// Check that GetBindAddressBkt handles all possible coin types
	// TODO -- do similar init checks for other switches over coinType
	for _, ct := range scanner.GetCoinTypes() {
		name := MustGetBindAddressBkt(ct)
		if len(name) == 0 {
			panic(fmt.Sprintf("GetBindAddressBkt(%s) returned empty", ct))
		}
	}
}

// Storer interface for exchange storage
type Storer interface {
	GetBindAddress(depositAddr, coinType string) (*BoundAddress, error)
	BindAddress(skyAddr, depositAddr, coinType, buyMethod string) (*BoundAddress, error)
	GetOrCreateDepositInfo(scanner.Deposit, string) (DepositInfo, error)
	GetDepositInfoArray(DepositFilter) ([]DepositInfo, error)
	GetDepositInfoOfSkyAddress(string) ([]DepositInfo, error)
	UpdateDepositInfo(string, func(DepositInfo) DepositInfo) (DepositInfo, error)
	UpdateDepositInfoCallback(string, func(DepositInfo) DepositInfo, func(DepositInfo) error) (DepositInfo, error)
	GetSkyBindAddresses(string) ([]BoundAddress, error)
	GetDepositStats() (int64, int64, error)
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
		if _, err := tx.CreateBucketIfNotExists(ExchangeMetaBkt); err != nil {
			return dbutil.NewCreateBucketFailedErr(ExchangeMetaBkt, err)
		}

		// create deposit status bucket if not exist
		if _, err := tx.CreateBucketIfNotExists(DepositInfoBkt); err != nil {
			return dbutil.NewCreateBucketFailedErr(DepositInfoBkt, err)
		}

		// create bind address bucket if not exist
		for _, ct := range scanner.GetCoinTypes() {
			bktName := MustGetBindAddressBkt(ct)
			if _, err := tx.CreateBucketIfNotExists(bktName); err != nil {
				return dbutil.NewCreateBucketFailedErr(bktName, err)
			}
		}

		if _, err := tx.CreateBucketIfNotExists(SkyDepositSeqsIndexBkt); err != nil {
			return dbutil.NewCreateBucketFailedErr(SkyDepositSeqsIndexBkt, err)
		}

		if _, err := tx.CreateBucketIfNotExists(BtcTxsBkt); err != nil {
			return dbutil.NewCreateBucketFailedErr(BtcTxsBkt, err)
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
func (s *Store) GetBindAddress(depositAddr, coinType string) (*BoundAddress, error) {
	var boundAddr *BoundAddress
	if err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		boundAddr, err = s.getBindAddressTx(tx, depositAddr, coinType)
		return err
	}); err != nil {
		return nil, err
	}

	return boundAddr, nil
}

// getBindAddressTx returns bound skycoin address of given bitcoin address.
// If no skycoin address is found, returns empty string and nil error.
func (s *Store) getBindAddressTx(tx *bolt.Tx, depositAddr, coinType string) (*BoundAddress, error) {
	bindBktFullName, err := GetBindAddressBkt(coinType)
	if err != nil {
		return nil, err
	}

	var boundAddr BoundAddress
	err = dbutil.GetBucketObject(tx, bindBktFullName, depositAddr, &boundAddr)
	switch err.(type) {
	case nil:
		return &boundAddr, nil
	case dbutil.ObjectNotExistErr:
		return nil, nil
	default:
		return nil, err
	}
}

// BindAddress binds a skycoin address to a deposit address
func (s *Store) BindAddress(skyAddr, depositAddr, coinType, buyMethod string) (*BoundAddress, error) {
	log := s.log.WithField("skyAddr", skyAddr)
	log = log.WithField("depositAddr", depositAddr)
	log = log.WithField("coinType", coinType)
	log = log.WithField("buyMethod", buyMethod)

	bindBktFullName, err := GetBindAddressBkt(coinType)
	if err != nil {
		return nil, err
	}

	boundAddr := BoundAddress{
		SkyAddress: skyAddr,
		Address:    depositAddr,
		CoinType:   coinType,
		BuyMethod:  buyMethod,
	}

	if err := s.db.Update(func(tx *bolt.Tx) error {
		existingSkyAddr, err := s.getBindAddressTx(tx, depositAddr, coinType)
		if err != nil {
			return err
		}

		if existingSkyAddr != nil {
			err := ErrAddressAlreadyBound
			log.WithError(err).Error("Attempted to bind an address twice")
			return err
		}

		// Update index of skycoin address and the deposit seq
		var addrs []BoundAddress
		if err := dbutil.GetBucketObject(tx, SkyDepositSeqsIndexBkt, skyAddr, &addrs); err != nil {
			switch err.(type) {
			case dbutil.ObjectNotExistErr:
			default:
				return err
			}
		}

		addrs = append(addrs, boundAddr)

		if err := dbutil.PutBucketValue(tx, SkyDepositSeqsIndexBkt, skyAddr, addrs); err != nil {
			return err
		}

		return dbutil.PutBucketValue(tx, bindBktFullName, depositAddr, boundAddr)
	}); err != nil {
		return nil, err
	}

	return &boundAddr, nil
}

// GetOrCreateDepositInfo creates a DepositInfo unless one exists with the DepositInfo.DepositID key,
// in which case it returns the existing DepositInfo.
func (s *Store) GetOrCreateDepositInfo(dv scanner.Deposit, rate string) (DepositInfo, error) {
	log := s.log.WithField("deposit", dv)
	log = log.WithField("rate", rate)

	var finalDepositInfo DepositInfo
	if err := s.db.Update(func(tx *bolt.Tx) error {
		di, err := s.getDepositInfoTx(tx, dv.ID())

		switch err.(type) {
		case nil:
			finalDepositInfo = di
			return nil

		case dbutil.ObjectNotExistErr:
			log.Info("DepositInfo not found in DB, inserting")
			boundAddr, err := s.getBindAddressTx(tx, dv.Address, dv.CoinType)
			if err != nil {
				err = fmt.Errorf("GetBindAddress failed: %v", err)
				log.WithError(err).Error(err)
				return err
			}

			if boundAddr == nil {
				err = ErrNoBoundAddress
				log.WithError(err).Error(err)
				return err
			}

			log = log.WithField("boundAddr", boundAddr)

			// Sanity check the boundAddr data against the deposit value data
			if boundAddr.CoinType != dv.CoinType {
				err := fmt.Errorf("boundAddr.CoinType != dv.CoinType")
				log.WithError(err).Error()
				return err
			}
			if boundAddr.Address != dv.Address {
				err := fmt.Errorf("boundAddr.Address != dv.Address")
				log.WithError(err).Error()
				return err
			}

			di := DepositInfo{
				CoinType:       dv.CoinType,
				DepositAddress: dv.Address,
				SkyAddress:     boundAddr.SkyAddress,
				BuyMethod:      boundAddr.BuyMethod,
				DepositID:      dv.ID(),
				Status:         StatusWaitDecide,
				DepositValue:   dv.Value,
				// Save the rate at the time this deposit was noticed
				ConversionRate: rate,
				Deposit:        dv,
			}

			log = log.WithField("depositInfo", di)

			updatedDi, err := s.addDepositInfoTx(tx, di)
			if err != nil {
				err = fmt.Errorf("addDepositInfoTx failed: %v", err)
				log.WithError(err).Error(err)
				return err
			}

			finalDepositInfo = updatedDi

			return nil

		default:
			err = fmt.Errorf("getDepositInfo failed: %v", err)
			log.WithError(err).Error(err)
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

	// check if the dpi with DepositID already exist
	if hasKey, err := dbutil.BucketHasKey(tx, DepositInfoBkt, di.DepositID); err != nil {
		return di, err
	} else if hasKey {
		return di, fmt.Errorf("deposit info of btctx \"%s\" already exists", di.DepositID)
	}

	seq, err := dbutil.NextSequence(tx, DepositInfoBkt)
	if err != nil {
		return di, err
	}

	updatedDi := di
	updatedDi.Seq = seq
	updatedDi.UpdatedAt = time.Now().UTC().Unix()

	if err := updatedDi.ValidateForStatus(); err != nil {
		log.WithError(err).Error("FIXME: Constructed invalid DepositInfo")
		return di, err
	}

	if err := dbutil.PutBucketValue(tx, DepositInfoBkt, updatedDi.DepositID, updatedDi); err != nil {
		return di, err
	}

	// update btc_txids bucket
	var txs []string
	if err := dbutil.GetBucketObject(tx, BtcTxsBkt, updatedDi.DepositAddress, &txs); err != nil {
		switch err.(type) {
		case dbutil.ObjectNotExistErr:
		default:
			return di, err
		}
	}

	txs = append(txs, updatedDi.DepositID)
	if err := dbutil.PutBucketValue(tx, BtcTxsBkt, updatedDi.DepositAddress, txs); err != nil {
		return di, err
	}

	return updatedDi, nil
}

// getDepositInfo returns depsoit info of given address
func (s *Store) getDepositInfo(btcTx string) (DepositInfo, error) {
	var di DepositInfo

	err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		di, err = s.getDepositInfoTx(tx, btcTx)
		return err
	})

	return di, err
}

// getDepositInfoTx returns depsoit info of given address
func (s *Store) getDepositInfoTx(tx *bolt.Tx, btcTx string) (DepositInfo, error) {
	var dpi DepositInfo

	if err := dbutil.GetBucketObject(tx, DepositInfoBkt, btcTx, &dpi); err != nil {
		return DepositInfo{}, err
	}

	return dpi, nil
}

// GetDepositInfoArray returns filtered deposit info
func (s *Store) GetDepositInfoArray(flt DepositFilter) ([]DepositInfo, error) {
	var dpis []DepositInfo

	if err := s.db.View(func(tx *bolt.Tx) error {
		return dbutil.ForEach(tx, DepositInfoBkt, func(k, v []byte) error {
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
		boundAddrs, err := s.getSkyBindAddressesTx(tx, skyAddr)
		if err != nil {
			return err
		}

		for _, boundAddr := range boundAddrs {
			var txns []string
			if err := dbutil.GetBucketObject(tx, BtcTxsBkt, boundAddr.Address, &txns); err != nil {
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
					Status:         StatusWaitDeposit,
					DepositAddress: boundAddr.Address,
					SkyAddress:     skyAddr,
					UpdatedAt:      time.Now().UTC().Unix(),
					CoinType:       boundAddr.CoinType,
				})
			}

			for _, txn := range txns {
				var dpi DepositInfo
				if err := dbutil.GetBucketObject(tx, DepositInfoBkt, txn, &dpi); err != nil {
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
	return s.UpdateDepositInfoCallback(btcTx, update, func(di DepositInfo) error { return nil })
}

// UpdateDepositInfoCallback updates deposit info. The update func takes a DepositInfo
// and returns a modified copy of it.  After updating the DepositInfo, it calls callback,
// inside of the transaction.  If the callback returns an error, the DepositInfo update
// is rolled back.
func (s *Store) UpdateDepositInfoCallback(btcTx string, update func(DepositInfo) DepositInfo, callback func(DepositInfo) error) (DepositInfo, error) {
	log := s.log.WithField("btcTx", btcTx)

	var dpi DepositInfo
	if err := s.db.Update(func(tx *bolt.Tx) error {
		if err := dbutil.GetBucketObject(tx, DepositInfoBkt, btcTx, &dpi); err != nil {
			return err
		}

		log = log.WithField("depositInfo", dpi)

		if dpi.DepositID != btcTx {
			log.Error("DepositInfo.DepositID does not match btcTx")
			err := fmt.Errorf("DepositInfo %+v saved under different key %s", dpi, btcTx)
			return err
		}

		dpi = update(dpi)
		dpi.UpdatedAt = time.Now().UTC().Unix()

		if err := dbutil.PutBucketValue(tx, DepositInfoBkt, btcTx, dpi); err != nil {
			return err
		}

		return callback(dpi)

	}); err != nil {
		return DepositInfo{}, err
	}

	return dpi, nil
}

// GetSkyBindAddresses returns the addresses of the given sky address bound
func (s *Store) GetSkyBindAddresses(skyAddr string) ([]BoundAddress, error) {
	var boundAddrs []BoundAddress

	if err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		boundAddrs, err = s.getSkyBindAddressesTx(tx, skyAddr)
		return err
	}); err != nil {
		return nil, err
	}

	return boundAddrs, nil
}

// getSkyBindAddressesTx returns the addresses of the given sky address bound
func (s *Store) getSkyBindAddressesTx(tx *bolt.Tx, skyAddr string) ([]BoundAddress, error) {
	var addrs []BoundAddress
	if err := dbutil.GetBucketObject(tx, SkyDepositSeqsIndexBkt, skyAddr, &addrs); err != nil {
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

// GetDepositStats returns BTC received and SKY sent
func (s *Store) GetDepositStats() (int64, int64, error) {
	var totalBTCReceived int64
	var totalSKYSent int64

	if err := s.db.View(func(tx *bolt.Tx) error {
		return dbutil.ForEach(tx, DepositInfoBkt, func(k, v []byte) error {
			var dpi DepositInfo
			if err := json.Unmarshal(v, &dpi); err != nil {
				return err
			}

			if dpi.CoinType == scanner.CoinTypeBTC {
				totalBTCReceived += dpi.DepositValue
			}
			totalSKYSent += int64(dpi.SkySent)

			return nil
		})
	}); err != nil {
		return -1, -1, err
	}

	return totalBTCReceived, totalSKYSent, nil
}
