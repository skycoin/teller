package addrs

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/testutil"
)

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

	btcAddrMgr, err := NewBTCAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Nil(t, err)
	require.NotNil(t, btcAddrMgr)
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

	expectedErr := errors.New("Invalid deposit address `bad`: Invalid address length")

	btcAddrMgr, err := NewBTCAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

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

	expectedErr := errors.New("Duplicate deposit address `14JwrdSxYXPxSi6crLKVwR4k2dbjfVZ3xj`")

	btcAddrMgr, err := NewBTCAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

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

	expectedErr := errors.New("No BTC addresses")

	btcAddrMgr, err := NewBTCAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, btcAddrMgr)
}

func TestNewBTCAddrsBadFormat(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := ``

	expectedErr := errors.New("Decode loaded address json failed: EOF")

	btcAddrMgr, err := NewBTCAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, btcAddrMgr)
}
