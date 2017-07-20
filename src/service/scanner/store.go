package scanner

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/boltdb/bolt"
)

var (
	scanMetaBkt         = []byte("scan_meta")
	lastScanBlockKey    = []byte("last_scan_block")
	depositAddressesKey = []byte("deposit_addresses")
)

// store records scanner meta info
type store struct {
	db *bolt.DB
}

func newStore(db *bolt.DB) (*store, error) {
	if db == nil {
		return nil, errors.New("new store failed: db is nil")
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		// create lastScanBlock bucket if not exist
		_, err := tx.CreateBucketIfNotExists(scanMetaBkt)
		if err != nil {
			return err
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &store{
		db: db,
	}, nil
}

// lastScanBlock struct in bucket
type lastScanBlock struct {
	Hash   string
	Height int32
}

// getLastScanBlock returns the last scanned block hash and height
func (s *store) getLastScanBlock() (string, int32, error) {
	var lsb lastScanBlock
	if err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(scanMetaBkt)
		if bkt == nil {
			return bucketNotExistErr(lastScanBlockKey)
		}

		if v := bkt.Get(lastScanBlockKey); v != nil {
			if err := json.Unmarshal(v, &lsb); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return "", 0, err
	}

	// if lsb.Hash == "" && lsb.Height == 0 {
	// 	return "", 0, nil
	// }

	// hash, err := chainhash.NewHashFromStr(lsb.Hash)
	// if err != nil {
	// 	return nil, 0, err
	// }

	return lsb.Hash, lsb.Height, nil
}

func (s *store) setLastScanBlock(b lastScanBlock) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(scanMetaBkt)
		if bkt == nil {
			return bucketNotExistErr(lastScanBlockKey)
		}
		v, err := json.Marshal(b)
		if err != nil {
			return err
		}

		return bkt.Put(lastScanBlockKey, v)
	})
}

func (s *store) getDepositAddresses() ([]string, error) {
	var addrs []string
	if err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(scanMetaBkt)
		if bucket == nil {
			return bucketNotExistErr(depositAddressesKey)
		}

		if v := bucket.Get(depositAddressesKey); v != nil {
			if err := json.Unmarshal(v, &addrs); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return []string{}, err
	}

	return addrs, nil
}

func (s *store) addDepositAddress(addr string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(scanMetaBkt)
		if bkt == nil {
			return bucketNotExistErr(depositAddressesKey)
		}
		var addrs []string
		if v := bkt.Get(depositAddressesKey); v != nil {
			if err := json.Unmarshal(v, &addrs); err != nil {
				return err
			}
		}
		for _, a := range addrs {
			if a == addr {
				return dupDepositAddrErr(addr)
			}
		}

		addrs = append(addrs, addr)
		v, err := json.Marshal(addrs)
		if err != nil {
			return err
		}

		return bkt.Put(depositAddressesKey, v)
	})
}

func bucketNotExistErr(bktName []byte) error {
	return fmt.Errorf("%s bucket does not exist", string(bktName))
}

func dupDepositAddrErr(addr string) error {
	return fmt.Errorf("deposit address %s already exist", addr)
}
