package testutil

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
	logrus_test "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/logger"
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
func NewLogger(t *testing.T) (*logrus.Logger, *logrus_test.Hook) {
	log, err := logger.NewLogger("", true)
	require.NoError(t, err)

	// Attach a log recorder for test inspection
	hook := logrus_test.NewLocal(log)

	return log, hook
}
