package addrs

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/testutil"
)

func TestNewETHAddrsNoFile(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	name := "doesnotexist.txt"
	_, err := NewETHAddrs(log, db, name)
	require.Error(t, err)
}

func TestNewETHAddrsLoadText(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesText := `0xc0a51efd9c319dd60d93105ab317eb362017ecb9
0x3f9f942b8bd4f69432c053eef77cd84fd46b8d76

0x5405f65a71342609249bb347505a4029c85ee88b
# ignore
0x01db29b6d512902aa82571267609f14187aa8aa8
`

	name := setupTempFile(t, addressesText)
	defer os.Remove(name)

	ethAddrMgr, err := NewETHAddrs(log, db, name)

	require.NoError(t, err)
	require.NotNil(t, ethAddrMgr)

	expectedAddrs := []string{
		"0xc0a51efd9c319dd60d93105ab317eb362017ecb9",
		"0x3f9f942b8bd4f69432c053eef77cd84fd46b8d76",
		"0x5405f65a71342609249bb347505a4029c85ee88b",
		"0x01db29b6d512902aa82571267609f14187aa8aa8",
	}

	require.Equal(t, expectedAddrs, ethAddrMgr.addresses)
}

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

	name := setupTempFile(t, addressesJSON)
	err := os.Rename(name, name+".json")
	require.NoError(t, err)
	defer os.Remove(name)

	ethAddrMgr, err := NewETHAddrs(log, db, name+".json")

	require.NoError(t, err)
	require.NotNil(t, ethAddrMgr)

	expectedAddrs := []string{
		"0xc0a51efd9c319dd60d93105ab317eb362017ecb9",
		"0x3f9f942b8bd4f69432c053eef77cd84fd46b8d76",
		"0x5405f65a71342609249bb347505a4029c85ee88b",
		"0x01db29b6d512902aa82571267609f14187aa8aa8",
	}

	require.Equal(t, expectedAddrs, ethAddrMgr.addresses)
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

	name := setupTempFile(t, addressesJSON)
	err := os.Rename(name, name+".json")
	require.NoError(t, err)
	defer os.Remove(name)

	expectedErr := errors.New("Invalid deposit address `bad`: Invalid address length")

	ethAddrMgr, err := NewETHAddrs(log, db, name+".json")

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

	name := setupTempFile(t, addressesJSON)
	err := os.Rename(name, name+".json")
	require.NoError(t, err)
	defer os.Remove(name)

	expectedErr := errors.New("Duplicate deposit address `0xc0a51efd9c319dd60d93105ab317eb362017ecb9`")

	ethAddrMgr, err := NewETHAddrs(log, db, name+".json")

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

	name := setupTempFile(t, addressesJSON)
	err := os.Rename(name, name+".json")
	require.NoError(t, err)
	defer os.Remove(name)

	expectedErr := errors.New("No ETH addresses")

	ethAddrMgr, err := NewETHAddrs(log, db, name+".json")

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, ethAddrMgr)
}

func TestNewETHAddrsBadFormat(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := ``

	name := setupTempFile(t, addressesJSON)
	err := os.Rename(name, name+".json")
	require.NoError(t, err)
	defer os.Remove(name)

	expectedErr := errors.New("Decode loaded address json failed: EOF")

	ethAddrMgr, err := NewETHAddrs(log, db, name+".json")

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, ethAddrMgr)
}
