package scanner

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"encoding/json"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/require"
)

func setupDB(t *testing.T) (*bolt.DB, func()) {
	rand.Seed(int64(time.Now().Second()))
	f := fmt.Sprintf("%s/test%d.db", os.TempDir(), rand.Intn(1024))
	db, err := bolt.Open(f, 0700, nil)
	require.Nil(t, err)
	return db, func() {
		db.Close()
		os.Remove(f)
	}
}

func TestNewStore(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(scanMetaBkt)
		require.NotNil(t, bkt)
		return nil
	})
}

func TestGetLastScanBlock(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	hash, height, err := s.getLastScanBlock()
	require.Nil(t, err)
	require.Equal(t, "", hash)
	require.Equal(t, int64(0), height)

	scanBlock := lastScanBlock{
		Hash:   "00000000000004509071260531df744090422d372d706cee907b2b5f2be8b8ff",
		Height: 222597,
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(scanMetaBkt)
		lsb := scanBlock

		v, err := json.Marshal(lsb)
		if err != nil {
			return err
		}

		return bkt.Put(lastScanBlockKey, v)
	})

	require.Nil(t, err)

	h1, height, err := s.getLastScanBlock()
	require.Nil(t, err)
	require.Equal(t, scanBlock.Hash, h1)
	require.Equal(t, scanBlock.Height, height)
}

func TestSetLastScanBlock(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	scanBlocks := []lastScanBlock{
		lastScanBlock{
			Hash:   "00000000000004509071260531df744090422d372d706cee907b2b5f2be8b8ff",
			Height: 222597,
		},
		lastScanBlock{
			Hash:   "000000000000003f499b9736635dd65101c4c70aef4912b5c5b4b86cd36b4d27",
			Height: 222618,
		},
	}

	require.Nil(t, s.setLastScanBlock(scanBlocks[0]))
	hash, height, err := s.getLastScanBlock()
	require.Nil(t, err)
	require.Equal(t, scanBlocks[0].Hash, hash)
	require.Equal(t, scanBlocks[0].Height, height)

	require.Nil(t, s.setLastScanBlock(scanBlocks[1]))
	hash, height, err = s.getLastScanBlock()
	require.Nil(t, err)
	require.Equal(t, scanBlocks[1].Hash, hash)
	require.Equal(t, scanBlocks[1].Height, height)
}

func TestGetDepositAddresses(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	var addrs = []string{
		"s1",
		"s2",
		"s3",
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(scanMetaBkt)
		v, err := json.Marshal(addrs)
		require.Nil(t, err)
		require.Nil(t, bkt.Put(depositAddressesKey, v))
		return nil
	})
	require.Nil(t, err)

	as, err := s.getDepositAddresses()
	require.Nil(t, err)
	require.Equal(t, addrs, as)

}

func TestAddDepositeAddress(t *testing.T) {
	addrs := []string{
		"a1",
		"a2",
		"a3",
		"a4",
	}

	var testCases = []struct {
		name        string
		initAddrs   []string
		addAddrs    []string
		expectAddrs []string
		err         error
	}{
		{
			"ok",
			addrs[:1],
			addrs[1:2],
			addrs[:2],
			nil,
		},
		{
			"dup",
			addrs[:2],
			addrs[1:2],
			[]string{},
			dupDepositAddrErr(addrs[1]),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, shutdown := setupDB(t)
			defer shutdown()
			s, err := newStore(db)
			require.Nil(t, err)

			err = db.Update(func(tx *bolt.Tx) error {
				v, err := json.Marshal(tc.initAddrs)
				require.Nil(t, err)
				return tx.Bucket(scanMetaBkt).Put(depositAddressesKey, v)
			})
			require.Nil(t, err)

			for _, a := range tc.addAddrs {
				if er := s.addDepositAddress(a); er != nil {
					err = er
				}
			}

			require.Equal(t, tc.err, err)
		})
	}
}
