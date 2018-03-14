package addrs

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MDLlife/teller/src/util/testutil"
)

func TestNewSKYAddrsAllValid(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `{
    "sky_addresses": [
		"CDLrMvPcmpdido8cbSFNNgzQXdC97TsgEQ",
		"fyqX5YuwXMUs4GEUE3LjLyhrqvNztFHQ4B",
		"2Dc7kXtwBLr8GL4TSZKFCJM3xqEwnqH6m67"
    ]
}`

	skyAddrMgr, err := NewSKYAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Nil(t, err)
	require.NotNil(t, skyAddrMgr)
}

func TestNewSKYAddrsContainsInvalid(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `{
    "sky_addresses": [
		"cBnu9sUvv12dovBmjQKTtfE4rbjMmf3fzW",
        "bad"
    ]
}`

	expectedErr := errors.New("Invalid deposit address `bad`: Invalid address length")

	skyAddrMgr, err := NewSKYAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, skyAddrMgr)
}

func TestNewSKYAddrsContainsDuplicated(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `{
      "sky_addresses": [
		"cBnu9sUvv12dovBmjQKTtfE4rbjMmf3fzW",
		"fyqX5YuwXMUs4GEUE3LjLyhrqvNztFHQ4B",
		"cBnu9sUvv12dovBmjQKTtfE4rbjMmf3fzW",
		"cBnu9sUvv12dovBmjQKTtfE4rbjMmf3fzW"
    ]
}`

	expectedErr := errors.New("Duplicate deposit address `cBnu9sUvv12dovBmjQKTtfE4rbjMmf3fzW`")

	skyAddrMgr, err := NewSKYAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, skyAddrMgr)
}

func TestNewSKYAddrsContainsNull(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `{
      "sky_addresses": []
}`

	expectedErr := errors.New("No SKY addresses")

	skyAddrMgr, err := NewSKYAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, skyAddrMgr)
}

func TestNewSKYAddrsBadFormat(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := ``

	expectedErr := errors.New("Decode loaded address json failed: EOF")

	skyAddrMgr, err := NewSKYAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, skyAddrMgr)
}
