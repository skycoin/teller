package btcaddrs

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/logger"
	"github.com/skycoin/teller/src/service/testutil"
)

func TestNewBtcAddrs(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	addrJSON := addressJSON{
		BtcAddresses: []string{
			"14JwrdSxYXPxSi6crLKVwR4k2dbjfVZ3xj",
			"1JNonvXRyZvZ4ZJ9PE8voyo67UQN1TpoGy",
			"1JrzSx8a9FVHHCkUFLB2CHULpbz4dTz5Ap",
		},
	}

	v, err := json.Marshal(addrJSON)
	require.NoError(t, err)

	btca, err := New(db, bytes.NewReader(v), logger.NewLogger("", true))
	require.NoError(t, err)

	addrMap := make(map[string]struct{}, len(btca.addresses))
	for _, a := range btca.addresses {
		addrMap[a] = struct{}{}
	}

	for _, addr := range addrJSON.BtcAddresses {
		_, ok := addrMap[addr]
		require.True(t, ok)
	}
}

func TestNewBtcAddrsContainsInvalid(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	invalidAddr := "invalid address"
	invalidAddrJSON := addressJSON{
		BtcAddresses: []string{
			"14JwrdSxYXPxSi6crLKVwR4k2dbjfVZ3xj",
			"1JNonvXRyZvZ4ZJ9PE8voyo67UQN1TpoGy",
			"1JrzSx8a9FVHHCkUFLB2CHULpbz4dTz5Ap",
			invalidAddr,
		},
	}

	v, err := json.Marshal(invalidAddrJSON)
	require.NoError(t, err)

	_, err = New(db, bytes.NewReader(v), logger.NewLogger("", true))
	require.Error(t, err)
	require.Equal(t, err, errors.New("Invalid address length"))
}

func TestNewAddress(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	addrJSON := addressJSON{
		BtcAddresses: []string{
			"14JwrdSxYXPxSi6crLKVwR4k2dbjfVZ3xj",
			"1JNonvXRyZvZ4ZJ9PE8voyo67UQN1TpoGy",
			"1JrzSx8a9FVHHCkUFLB2CHULpbz4dTz5Ap",
			"1JrzSx8a9FVHHCkUFLB2CHULpbz4dTz5Ap",
		},
	}

	v, err := json.Marshal(addrJSON)
	require.NoError(t, err)

	btca, err := New(db, bytes.NewReader(v), logger.NewLogger("", true))
	require.NoError(t, err)

	addr, err := btca.NewAddress()
	require.NoError(t, err)

	addrMap := make(map[string]struct{})
	for _, a := range btca.addresses {
		addrMap[a] = struct{}{}
	}

	// check if the addr still in the address pool
	_, ok := addrMap[addr]
	require.False(t, ok)

	// check if the addr is in used storage
	exists, err := btca.used.IsExist(addr)
	require.NoError(t, err)
	require.True(t, exists)

	btca1, err := New(db, bytes.NewReader(v), logger.NewLogger("", true))
	require.NoError(t, err)

	addrMap1 := make(map[string]struct{})
	for _, a := range btca1.addresses {
		addrMap1[a] = struct{}{}
	}
	_, ok = addrMap1[addr]
	require.False(t, ok)

	exists, err = btca1.used.IsExist(addr)
	require.True(t, exists)

	// run out all addresses
	for i := 0; i < 2; i++ {
		btca1.NewAddress()
	}

	_, err = btca1.NewAddress()
	require.Equal(t, ErrDepositAddressEmpty, err)
}
