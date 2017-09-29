package btcaddrs

import (
	"testing"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/service/testutil"
)

func TestNewStore(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)
	require.NotNil(t, s.cache)

	db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(usedAddrBkt)
		require.NotNil(t, bkt)
		return nil
	})
}

func TestLoadCache(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	db.Update(func(tx *bolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists(usedAddrBkt)
		require.Nil(t, err)
		require.NotNil(t, bkt)

		bkt.Put([]byte("a1"), []byte(""))
		bkt.Put([]byte("a2"), []byte(""))
		bkt.Put([]byte("a3"), []byte(""))

		return nil
	})

	s, err := newStore(db)
	require.Nil(t, err)

	_, ok := s.cache["a1"]
	require.True(t, ok)

	_, ok = s.cache["a1"]
	require.True(t, ok)
}

func TestStorePut(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	require.Nil(t, s.Put("a1"))
	require.Nil(t, s.Put("a2"))

	db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(usedAddrBkt)
		require.NotNil(t, bkt)
		require.NotNil(t, bkt.Get([]byte("a1")))
		require.NotNil(t, bkt.Get([]byte("a2")))
		return nil
	})

	_, ok := s.cache["a1"]
	require.True(t, ok)
	_, ok = s.cache["a2"]
	require.True(t, ok)
}

func TestStoreGet(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(usedAddrBkt)
		require.NotNil(t, bkt)

		bkt.Put([]byte("a1"), []byte(""))
		bkt.Put([]byte("a2"), []byte(""))

		s.cache["a1"] = struct{}{}
		s.cache["a2"] = struct{}{}
		return nil
	})

	require.True(t, s.IsExsit("a1"))
	require.True(t, s.IsExsit("a2"))
	require.False(t, s.IsExsit("a3"))
}
