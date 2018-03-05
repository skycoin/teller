package addrs

import (
	"testing"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/require"

	"github.com/MDLlife/teller/src/util/dbutil"
	"github.com/MDLlife/teller/src/util/testutil"
)

func TestNewStore(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := NewStore(db, "test_bucket")
	require.NoError(t, err)

	err = db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(s.BucketKey)
		require.NotNil(t, bkt)
		return nil
	})
	require.NoError(t, err)
}

func TestStorePut(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := NewStore(db, "test_bucket")
	require.NoError(t, err)

	require.Nil(t, s.Put("a1"))
	require.Nil(t, s.Put("a2"))

	err = db.View(func(tx *bolt.Tx) error {
		used, err := dbutil.BucketHasKey(tx, s.BucketKey, "a1")
		require.NoError(t, err)
		if err != nil {
			return err
		}
		require.True(t, used)

		used, err = dbutil.BucketHasKey(tx, s.BucketKey, "a2")
		require.NoError(t, err)
		require.True(t, used)

		return err
	})
	require.NoError(t, err)
}

func TestStoreGet(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := NewStore(db, "test_bucket")
	require.NoError(t, err)

	err = db.Update(func(tx *bolt.Tx) error {
		err := dbutil.PutBucketValue(tx, s.BucketKey, "a1", "")
		require.NoError(t, err)
		if err != nil {
			return err
		}

		err = dbutil.PutBucketValue(tx, s.BucketKey, "a2", "")
		require.NoError(t, err)
		return err
	})
	require.NoError(t, err)

	used, err := s.IsUsed("a1")
	require.NoError(t, err)
	require.True(t, used)

	used, err = s.IsUsed("a2")
	require.NoError(t, err)
	require.True(t, used)

	used, err = s.IsUsed("a3")
	require.NoError(t, err)
	require.False(t, used)
}
