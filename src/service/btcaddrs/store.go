package btcaddrs

import (
	"github.com/boltdb/bolt"
	"github.com/skycoin/teller/src/util/dbutil"
)

var usedAddrBkt = []byte("used_btc_address")

type store struct {
	db *bolt.DB
}

func newStore(db *bolt.DB) (*store, error) {
	// creates usedAddressBkt if not exist
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(usedAddrBkt)
		return err
	}); err != nil {
		return nil, err
	}

	return &store{
		db: db,
	}, nil
}

func (s *store) Put(addr string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(usedAddrBkt).Put([]byte(addr), []byte(""))
	})
}

// checks if address is mark as used
func (s *store) IsExist(addr string) (bool, error) {
	exists := false
	if err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		exists, err = dbutil.BucketHasKey(tx, usedAddrBkt, addr)
		return err
	}); err != nil {
		return false, err
	}

	return exists, nil
}
