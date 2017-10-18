package addrs

import (
	"errors"

	"github.com/boltdb/bolt"
	"github.com/skycoin/teller/src/util/dbutil"
)

// Store saves used addresses in a bucket
type Store struct {
	db        *bolt.DB
	BucketKey []byte
}

// NewStore creates a Store for a bucket key
func NewStore(db *bolt.DB, key string) (*Store, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}

	// creates usedAddressBkt if not exist
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(key))
		return err
	}); err != nil {
		return nil, err
	}

	return &Store{
		db:        db,
		BucketKey: []byte(key),
	}, nil
}

// Put sets an address in the bucket, marking it as used
func (s *Store) Put(addr string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(s.BucketKey).Put([]byte(addr), []byte(""))
	})
}

// Get all keys from the bucket
func (s *Store) GetAll() ([]string, error) {
	var addrs []string
	s.db.View(func(tx *bolt.Tx) error {
		tx.Bucket(s.BucketKey).ForEach(func(k, v []byte) error {
			s := string(k[:])
			addrs = append(addrs, s)
			return nil
		})
		return nil
	})
	return addrs, nil
}

// IsUsed checks if address is mark as used
func (s *Store) IsUsed(addr string) (bool, error) {
	exists := false
	if err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		exists, err = dbutil.BucketHasKey(tx, s.BucketKey, addr)
		return err
	}); err != nil {
		return false, err
	}

	return exists, nil
}
