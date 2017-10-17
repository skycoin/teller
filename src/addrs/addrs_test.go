package addrs

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/testutil"
)

func TestNewBtcAddrs(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	addresses := []string{
		"14JwrdSxYXPxSi6crLKVwR4k2dbjfVZ3xj",
		"1JNonvXRyZvZ4ZJ9PE8voyo67UQN1TpoGy",
		"1JrzSx8a9FVHHCkUFLB2CHULpbz4dTz5Ap",
	}

	log, _ := testutil.NewLogger(t)
	btca, err := NewAddrs(log, db, addresses, "test_bucket")
	require.NoError(t, err)

	addrMap := make(map[string]struct{}, len(btca.addresses))
	for _, a := range btca.addresses {
		addrMap[a] = struct{}{}
	}

	for _, addr := range addresses {
		_, ok := addrMap[addr]
		require.True(t, ok)
	}
}

func TestNewAddress(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	addresses := []string{
		"14JwrdSxYXPxSi6crLKVwR4k2dbjfVZ3xj",
		"1JNonvXRyZvZ4ZJ9PE8voyo67UQN1TpoGy",
		"1JrzSx8a9FVHHCkUFLB2CHULpbz4dTz5Ap",
		"1JrzSx8a9FVHHCkUFLB2CHULpbz4dTz5Ap",
	}

	log, _ := testutil.NewLogger(t)
	btca, err := NewAddrs(log, db, addresses, "test_bucket")
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
	used, err := btca.used.IsUsed(addr)
	require.NoError(t, err)
	require.True(t, used)

	log, _ := testutil.NewLogger(t)
	btca1, err := NewAddrs(log, db, addresses, "test_bucket")
	require.NoError(t, err)

	for _, a := range btca1.addresses {
		require.NotEqual(t, a, addr)
	}

	used, err = btca1.used.IsUsed(addr)
	require.True(t, used)

	// run out all addresses
	for i := 0; i < 2; i++ {
		_, err = btca1.NewAddress()
		require.NoError(t, err)
	}

	_, err = btca1.NewAddress()
	require.Error(t, err)
	require.Equal(t, ErrDepositAddressEmpty, err)
}
