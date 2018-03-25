package addrs

import (
	"errors"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/testutil"
)

func setupTempFile(t *testing.T, body string) string {
	f, err := ioutil.TempFile("", "addrs-test-")
	name := f.Name()
	require.NoError(t, err)
	_, err = f.Write([]byte(body))
	require.NoError(t, err)
	err = f.Close()
	require.NoError(t, err)
	return name
}

func TestNewBTCAddrsNoFile(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	name := "doesnotexist.txt"
	_, err := NewBTCAddrs(log, db, name)
	require.Error(t, err)
}

func TestNewBTCAddrsLoadText(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesText := `1PZ63K3G4gZP6A6E2TTbBwxT5bFQGL2TLB
14FG8vQnmK6B7YbLSr6uC5wfGY78JFNCYg

1Mv16pwUZYUrMWLTe2DDZzXHGAyHdKA5oz
# ignore
1NvBwUKqUuH3HbPjHq417XhQ551RHhogso
1Kar4VK9HLkcQ99iWbs4LuCGEyDdTab5PC
`

	name := setupTempFile(t, addressesText)
	defer func() {
		err := os.Remove(name)
		require.NoError(t, err)
	}()

	btcAddrMgr, err := NewBTCAddrs(log, db, name)

	require.NoError(t, err)
	require.NotNil(t, btcAddrMgr)

	expectedAddrs := []string{
		"1PZ63K3G4gZP6A6E2TTbBwxT5bFQGL2TLB",
		"14FG8vQnmK6B7YbLSr6uC5wfGY78JFNCYg",
		"1Mv16pwUZYUrMWLTe2DDZzXHGAyHdKA5oz",
		"1NvBwUKqUuH3HbPjHq417XhQ551RHhogso",
		"1Kar4VK9HLkcQ99iWbs4LuCGEyDdTab5PC",
	}

	require.Equal(t, expectedAddrs, btcAddrMgr.addresses)
}

func TestNewBTCAddrsAllValid(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `{
    "btc_addresses": [
        "1PZ63K3G4gZP6A6E2TTbBwxT5bFQGL2TLB",
        "14FG8vQnmK6B7YbLSr6uC5wfGY78JFNCYg",
        "1Mv16pwUZYUrMWLTe2DDZzXHGAyHdKA5oz",
        "1NvBwUKqUuH3HbPjHq417XhQ551RHhogso",
        "1Kar4VK9HLkcQ99iWbs4LuCGEyDdTab5PC"
    ]
}`

	name := setupTempFile(t, addressesJSON)
	err := os.Rename(name, name+".json")
	require.NoError(t, err)
	defer func() {
		err := os.Remove(name + ".json")
		require.NoError(t, err)
	}()

	btcAddrMgr, err := NewBTCAddrs(log, db, name+".json")

	require.NoError(t, err)
	require.NotNil(t, btcAddrMgr)

	expectedAddrs := []string{
		"1PZ63K3G4gZP6A6E2TTbBwxT5bFQGL2TLB",
		"14FG8vQnmK6B7YbLSr6uC5wfGY78JFNCYg",
		"1Mv16pwUZYUrMWLTe2DDZzXHGAyHdKA5oz",
		"1NvBwUKqUuH3HbPjHq417XhQ551RHhogso",
		"1Kar4VK9HLkcQ99iWbs4LuCGEyDdTab5PC",
	}

	require.Equal(t, expectedAddrs, btcAddrMgr.addresses)
}

func TestNewBtcAddrsContainsInvalid(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `{
    "btc_addresses": [
        "14JwrdSxYXPxSi6crLKVwR4k2dbjfVZ3xj",
        "1JNonvXRyZvZ4ZJ9PE8voyo67UQN1TpoGy",
        "1JrzSx8a9FVHHCkUFLB2CHULpbz4dTz5Ap",
        "bad"
    ]
}`

	name := setupTempFile(t, addressesJSON)
	err := os.Rename(name, name+".json")
	require.NoError(t, err)
	defer func() {
		err := os.Remove(name + ".json")
		require.NoError(t, err)
	}()

	expectedErr := errors.New("Invalid deposit address `bad`: Invalid address length")

	btcAddrMgr, err := NewBTCAddrs(log, db, name+".json")

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, btcAddrMgr)
}

func TestNewBtcAddrsContainsDuplicated(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `{
      "btc_addresses": [
        "14JwrdSxYXPxSi6crLKVwR4k2dbjfVZ3xj",
        "1JNonvXRyZvZ4ZJ9PE8voyo67UQN1TpoGy",
        "14JwrdSxYXPxSi6crLKVwR4k2dbjfVZ3xj",
        "1JrzSx8a9FVHHCkUFLB2CHULpbz4dTz5Ap"
    ]
}`

	name := setupTempFile(t, addressesJSON)
	err := os.Rename(name, name+".json")
	require.NoError(t, err)
	defer func() {
		err := os.Remove(name + ".json")
		require.NoError(t, err)
	}()

	expectedErr := errors.New("Duplicate deposit address `14JwrdSxYXPxSi6crLKVwR4k2dbjfVZ3xj`")

	btcAddrMgr, err := NewBTCAddrs(log, db, name+".json")

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, btcAddrMgr)
}

func TestNewBTCAddrsContainsNull(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `{
      "btc_addresses": []
}`

	name := setupTempFile(t, addressesJSON)
	err := os.Rename(name, name+".json")
	require.NoError(t, err)
	defer func() {
		err := os.Remove(name + ".json")
		require.NoError(t, err)
	}()

	expectedErr := errors.New("No BTC addresses")

	btcAddrMgr, err := NewBTCAddrs(log, db, name+".json")

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, btcAddrMgr)
}

func TestNewBTCAddrsBadFormat(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := ``

	name := setupTempFile(t, addressesJSON)
	err := os.Rename(name, name+".json")
	require.NoError(t, err)
	defer func() {
		err := os.Remove(name + ".json")
		require.NoError(t, err)
	}()

	expectedErr := errors.New("Decode loaded address json failed: EOF")

	btcAddrMgr, err := NewBTCAddrs(log, db, name+".json")

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, btcAddrMgr)
}
