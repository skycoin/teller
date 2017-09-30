package testutil

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/require"
)

// PrepareDB initializes a temporary bolt.DB
func PrepareDB(t *testing.T) (*bolt.DB, func()) {
	f, err := ioutil.TempFile("", "testdb")
	require.Nil(t, err)

	db, err := bolt.Open(f.Name(), 0700, nil)
	require.Nil(t, err)

	return db, func() {
		db.Close()
		os.Remove(f.Name())
	}
}
