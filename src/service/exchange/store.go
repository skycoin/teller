package exchange

import (
	"errors"
	"log"
	"strings"
	"time"

	"fmt"

	"encoding/json"

	"sort"

	"github.com/boltdb/bolt"
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
)

// store storage for exchange
type store struct {
	db *bolt.DB
}

// newStore creates a store instance
func newStore(db *bolt.DB) (*store, error) {
	if db == nil {
		return nil, errors.New("new exchange store failed, db is nil")
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

	return &store{
		db: db,
	}, nil
}

// GetBindAddress returns bound skycoin address of given bitcoin address.
// If no skycoin address is found, returns empty string and nil error.
func (s *store) GetBindAddress(btcAddr string) (string, error) {
	skyAddr := ""

	if err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		skyAddr, err = dbutil.GetBucketString(tx, bindAddressBkt, btcAddr)

		switch err.(type) {
		case nil:
			return nil
		case dbutil.ObjectNotExistErr:
			return nil
		default:
			return err
		}
	}); err != nil {
		return "", err
	}

	return skyAddr, nil
}

func (s *store) BindAddress(skyAddr, btcAddr string) error {
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
		// check if the dpi with BtcTx already exist
		if hasKey, err := dbutil.BucketHasKey(tx, depositInfoBkt, dpinfo.BtcTx); err != nil {
			return err
		} else if hasKey {
			return fmt.Errorf("deposit info of btctx %s already exists", dpinfo.BtcTx)
		}

		seq, err := dbutil.NextSequence(tx, depositInfoBkt)
		if err != nil {
			return err
		}

		dpinfo.Seq = seq
		dpinfo.UpdatedAt = time.Now().UTC().Unix()

		if err := dbutil.PutBucketValue(tx, depositInfoBkt, dpinfo.BtcTx, dpinfo); err != nil {
			return err
		}

		// update btc_txids bucket
		var txs []string
		if err := dbutil.GetBucketObject(tx, btcTxsBkt, dpinfo.BtcAddress, &txs); err != nil {
			switch err.(type) {
			case dbutil.ObjectNotExistErr:
			default:
				return err
			}
		}

		txs = append(txs, dpinfo.BtcTx)
		return dbutil.PutBucketValue(tx, btcTxsBkt, dpinfo.BtcAddress, txs)
	})
}

// GetDepositInfo returns depsoit info of given btc address
func (s *store) GetDepositInfo(btcTx string) (DepositInfo, error) {
	var dpi DepositInfo

	if err := s.db.View(func(tx *bolt.Tx) error {
		return dbutil.GetBucketObject(tx, depositInfoBkt, btcTx, &dpi)
	}); err != nil {
		return DepositInfo{}, err
	}

	return dpi, nil
}

// GetDepositInfoArray returns filtered deposit info
func (s *store) GetDepositInfoArray(flt DepositFilter) ([]DepositInfo, error) {
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

// UpdateDepositInfo updates deposit info. The update func takes a DepositInfo
// and returns a modified copy of it.
func (s *store) UpdateDepositInfo(btcTx string, update func(DepositInfo) DepositInfo) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		var dpi DepositInfo
		if err := dbutil.GetBucketObject(tx, depositInfoBkt, btcTx, &dpi); err != nil {
			return err
		}

		if dpi.BtcTx != btcTx {
			err := fmt.Errorf("DepositInfo %+v saved under different key %s", dpi, btcTx)
			log.Printf("ERROR: %v\n", err)
			return err
		}

		dpi = update(dpi)
		dpi.UpdatedAt = time.Now().UTC().Unix()

		return dbutil.PutBucketValue(tx, depositInfoBkt, btcTx, dpi)
	})
}

// GetDepositInfoOfSkyAddress returns all deposit info that are bound
// to the given skycoin address
func (s *store) GetDepositInfoOfSkyAddress(skyAddr string) ([]DepositInfo, error) {
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
					// TODO: Why is this default DepositInfo included?
					dpis = append(dpis, DepositInfo{
						BtcAddress: btcAddr,
						SkyAddress: skyAddr,
						UpdatedAt:  time.Now().UTC().Unix(),
					})
					continue
				default:
					return err
				}
			}

			if len(txns) == 0 {
				// TODO: Why is this default DepositInfo included?
				dpis = append(dpis, DepositInfo{
					BtcAddress: btcAddr,
					SkyAddress: skyAddr,
					UpdatedAt:  time.Now().UTC().Unix(),
				})

				continue
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

// GetSkyBindBtcAddresses returns the btc addresses of the given sky address bound
func (s *store) GetSkyBindBtcAddresses(skyAddr string) ([]string, error) {
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
func (s *store) GetSkyBindBtcAddressesTx(tx *bolt.Tx, skyAddr string) ([]string, error) {
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
