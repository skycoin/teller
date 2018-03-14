package addrs

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/testutil"
)

func TestNewSKYAddrsAllValid(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	// just some random addresses from the explorer
	addressesJSON := `{
    "sky_addresses": [
		"2dQkJ2YMEaXoeCcuJnqLiWJEh7xnLx1btqF",
		"qboYMqpEJVcRbLkvXXL9dZKinGt5AYYB6j",
		"3W7xXw33PBe7YyAJ9TGoc9MKw5uaMMYots",
		"3BhjEkZ5rFaXqQZr44msr8nYRPiV7tcLQs"
    ]
}`

	skyAddrMgr, err := NewSKYAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Nil(t, err)
	require.NotNil(t, skyAddrMgr)
}

func TestNewSkyAddrsContainsInvalid(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `{
    "sky_addresses": [
		"2dQkJ2YMEaXoeCcuJnqLiWJEh7xnLx1btqF",
		"qboYMqpEJVcRbLkvXXL9dZKinGt5AYYB6j",
		"3W7xXw33PBe7YyAJ9TGoc9MKw5uaMMYots",
		"bad"
    ]
}`

	expectedErr := errors.New("Invalid deposit address `bad`: Invalid address length")

	skyAddrMgr, err := NewSKYAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, skyAddrMgr)
}

func TestNewSkyAddrsContainsDuplicated(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `{
      "sky_addresses": [
		"3W7xXw33PBe7YyAJ9TGoc9MKw5uaMMYots",
		"qboYMqpEJVcRbLkvXXL9dZKinGt5AYYB6j",
		"3W7xXw33PBe7YyAJ9TGoc9MKw5uaMMYots",
		"3BhjEkZ5rFaXqQZr44msr8nYRPiV7tcLQs",
		"bad"
    ]
}`

	expectedErr := errors.New("Duplicate deposit address `3W7xXw33PBe7YyAJ9TGoc9MKw5uaMMYots`")

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

	ethAddrMgr, err := NewSKYAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, ethAddrMgr)
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
