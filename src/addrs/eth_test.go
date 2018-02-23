package addrs

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/testutil"
)

func TestNewETHAddrsAllValid(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `{
    "eth_addresses": [
		"0xc0a51efd9c319dd60d93105ab317eb362017ecb9",
		"0x3f9f942b8bd4f69432c053eef77cd84fd46b8d76",
		"0x5405f65a71342609249bb347505a4029c85ee88b",
		"0x01db29b6d512902aa82571267609f14187aa8aa8"
    ]
}`

	ethAddrMgr, err := NewETHAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Nil(t, err)
	require.NotNil(t, ethAddrMgr)
}

func TestNewEthAddrsContainsInvalid(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `{
    "eth_addresses": [
		"0xc0a51efd9c319dd60d93105ab317eb362017ecb9",
		"0x3f9f942b8bd4f69432c053eef77cd84fd46b8d76",
		"0x5405f65a71342609249bb347505a4029c85ee88b",
		"0x01db29b6d512902aa82571267609f14187aa8aa8",
        "bad"
    ]
}`

	expectedErr := errors.New("Invalid deposit address `bad`: Invalid address length")

	ethAddrMgr, err := NewETHAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, ethAddrMgr)
}

func TestNewEthAddrsContainsDuplicated(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `{
      "eth_addresses": [
		"0xc0a51efd9c319dd60d93105ab317eb362017ecb9",
		"0x3f9f942b8bd4f69432c053eef77cd84fd46b8d76",
		"0xc0a51efd9c319dd60d93105ab317eb362017ecb9",
		"0x01db29b6d512902aa82571267609f14187aa8aa8"
    ]
}`

	expectedErr := errors.New("Duplicate deposit address `0xc0a51efd9c319dd60d93105ab317eb362017ecb9`")

	ethAddrMgr, err := NewETHAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, ethAddrMgr)
}

func TestNewETHAddrsContainsNull(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `{
      "eth_addresses": []
}`

	expectedErr := errors.New("No ETH addresses")

	ethAddrMgr, err := NewETHAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, ethAddrMgr)
}

func TestNewETHAddrsBadFormat(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := ``

	expectedErr := errors.New("Decode loaded address json failed: EOF")

	ethAddrMgr, err := NewETHAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, ethAddrMgr)
}
