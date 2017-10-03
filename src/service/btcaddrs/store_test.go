package btcaddrs

import (
	"testing"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/dbutil"
	"github.com/skycoin/teller/src/service/testutil"
)

func TestNewStore(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	_, err := newStore(db)
	require.NoError(t, err)

	err = db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(usedAddrBkt)
		require.NotNil(t, bkt)
		return nil
	})
	require.NoError(t, err)
}

func TestStorePut(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.NoError(t, err)

	require.Nil(t, s.Put("a1"))
	require.Nil(t, s.Put("a2"))

	err = db.View(func(tx *bolt.Tx) error {
		exists, err := dbutil.BucketHasKey(tx, usedAddrBkt, "a1")
		require.NoError(t, err)
		if err != nil {
			return err
		}
		require.True(t, exists)

		exists, err = dbutil.BucketHasKey(tx, usedAddrBkt, "a2")
		require.NoError(t, err)
		require.True(t, exists)

		return err
	})
	require.NoError(t, err)
}

func TestStoreGet(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.NoError(t, err)

	err = db.Update(func(tx *bolt.Tx) error {
		err := dbutil.PutBucketValue(tx, usedAddrBkt, "a1", "")
		require.NoError(t, err)
		if err != nil {
			return err
		}

		err = dbutil.PutBucketValue(tx, usedAddrBkt, "a2", "")
		require.NoError(t, err)
		return err
	})
	require.NoError(t, err)

	exists, err := s.IsExist("a1")
	require.NoError(t, err)
	require.True(t, exists)

	exists, err = s.IsExist("a2")
	require.NoError(t, err)
	require.True(t, exists)

	exists, err = s.IsExist("a3")
	require.NoError(t, err)
	require.False(t, exists)
}
