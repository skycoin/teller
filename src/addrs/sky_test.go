package addrs

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/testutil"
)

func TestNewSKYAddrsNoFile(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	name := "doesnotexist.txt"
	_, err := NewSKYAddrs(log, db, name)
	require.Error(t, err)
}

func TestNewSKYAddrsLoadText(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesText := `2dQkJ2YMEaXoeCcuJnqLiWJEh7xnLx1btqF
qboYMqpEJVcRbLkvXXL9dZKinGt5AYYB6j

3W7xXw33PBe7YyAJ9TGoc9MKw5uaMMYots
# ignore
3BhjEkZ5rFaXqQZr44msr8nYRPiV7tcLQs
`

	name := setupTempFile(t, addressesText)
	defer func() {
		err := os.Remove(name)
		require.NoError(t, err)
	}()

	skyAddrMgr, err := NewSKYAddrs(log, db, name)

	require.NoError(t, err)
	require.NotNil(t, skyAddrMgr)

	expectedAddrs := []string{
		"2dQkJ2YMEaXoeCcuJnqLiWJEh7xnLx1btqF",
		"qboYMqpEJVcRbLkvXXL9dZKinGt5AYYB6j",
		"3W7xXw33PBe7YyAJ9TGoc9MKw5uaMMYots",
		"3BhjEkZ5rFaXqQZr44msr8nYRPiV7tcLQs",
	}

	require.Equal(t, expectedAddrs, skyAddrMgr.addresses)
}

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

	name := setupTempFile(t, addressesJSON)
	err := os.Rename(name, name+".json")
	require.NoError(t, err)
	defer func() {
		err := os.Remove(name + ".json")
		require.NoError(t, err)
	}()

	skyAddrMgr, err := NewSKYAddrs(log, db, name+".json")

	require.NoError(t, err)
	require.NotNil(t, skyAddrMgr)

	expectedAddrs := []string{
		"2dQkJ2YMEaXoeCcuJnqLiWJEh7xnLx1btqF",
		"qboYMqpEJVcRbLkvXXL9dZKinGt5AYYB6j",
		"3W7xXw33PBe7YyAJ9TGoc9MKw5uaMMYots",
		"3BhjEkZ5rFaXqQZr44msr8nYRPiV7tcLQs",
	}

	require.Equal(t, expectedAddrs, skyAddrMgr.addresses)
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

	name := setupTempFile(t, addressesJSON)
	err := os.Rename(name, name+".json")
	require.NoError(t, err)
	defer func() {
		err := os.Remove(name + ".json")
		require.NoError(t, err)
	}()

	expectedErr := errors.New("Invalid deposit address `bad`: Invalid address length")

	skyAddrMgr, err := NewSKYAddrs(log, db, name+".json")

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
		"3BhjEkZ5rFaXqQZr44msr8nYRPiV7tcLQs"
    ]
}`

	name := setupTempFile(t, addressesJSON)
	err := os.Rename(name, name+".json")
	require.NoError(t, err)
	defer func() {
		err := os.Remove(name + ".json")
		require.NoError(t, err)
	}()

	expectedErr := errors.New("Duplicate deposit address `3W7xXw33PBe7YyAJ9TGoc9MKw5uaMMYots`")

	skyAddrMgr, err := NewSKYAddrs(log, db, name+".json")

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

	name := setupTempFile(t, addressesJSON)
	err := os.Rename(name, name+".json")
	require.NoError(t, err)
	defer func() {
		err := os.Remove(name + ".json")
		require.NoError(t, err)
	}()

	expectedErr := errors.New("No SKY addresses")

	skyAddrMgr, err := NewSKYAddrs(log, db, name+".json")

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, skyAddrMgr)
}

func TestNewSKYAddrsBadFormat(t *testing.T) {
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

	skyAddrMgr, err := NewSKYAddrs(log, db, name+".json")

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, skyAddrMgr)
}
