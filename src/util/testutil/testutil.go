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
	require.NoError(t, err)

	db, err := bolt.Open(f.Name(), 0700, nil)
	require.NoError(t, err)

	return db, func() {
		err := db.Close()
		require.NoError(t, err)
		err = os.Remove(f.Name())
		require.NoError(t, err)
	}
}

// CheckError calls f and asserts it did not return an error
func CheckError(t *testing.T, f func() error) {
	t.Helper()
	err := f()
	require.NoError(t, err)
}

// NewLogger returns a logger that only writes to stdout and with debug level
func NewLogger(t *testing.T) (*logrus.Logger, *logrus_test.Hook) {
	log, err := logger.NewLogger("", true)
	require.NoError(t, err)

	// Attach a log recorder for test inspection
	hook := logrus_test.NewLocal(log)

	return log, hook
}
