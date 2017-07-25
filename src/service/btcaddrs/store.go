package btcaddrs

import (
	"sync"

	"github.com/boltdb/bolt"
)

var usedAddrBkt = []byte("used_btc_address")

type store struct {
	sync.Mutex
	db    *bolt.DB
	cache map[string]struct{} // cache the used address
}

func newStore(db *bolt.DB) (*store, error) {
	// creates usedAddressBkt if not exist
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(usedAddrBkt)
		return err
	}); err != nil {
		return nil, err
	}

	s := &store{
		db:    db,
		cache: make(map[string]struct{}),
	}

	if err := s.loadCache(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *store) loadCache() error {
	return s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(usedAddrBkt)
		bkt.ForEach(func(k []byte, v []byte) error {
			s.cache[string(k)] = struct{}{}
			return nil
		})
		return nil
	})
}

func (s *store) Put(addr string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(usedAddrBkt).Put([]byte(addr), []byte("")); err != nil {
			return err
		}
		s.Lock()
		s.cache[addr] = struct{}{}
		s.Unlock()
		return nil
	})
}

// checks if address is mark as used
func (s *store) IsExsit(addr string) bool {
	s.Lock()
	defer s.Unlock()
	_, ok := s.cache[addr]
	return ok
}
