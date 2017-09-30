package btcaddrs

import (
	"github.com/boltdb/bolt"
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
	var v []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		v = tx.Bucket(usedAddrBkt).Get(addr)
		return nil
	}); err != nil {
		return false, err
	}

	return v != nil, nil
}
