package testutil

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/logger"
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

// NewLogger returns a logger that only writes to stdout and with debug level
func NewLogger(t *testing.T) *logrus.Logger {
	log, err := logger.NewLogger("", true)
	require.NoError(t, err)
	return log
}
