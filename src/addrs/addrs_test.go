package addrs

import (
	"testing"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/testutil"
)

func testNewBtcAddrManager(t *testing.T, db *bolt.DB, log *logrus.Logger) (*Addrs, []string) {
	addresses := []string{
		"14JwrdSxYXPxSi6crLKVwR4k2dbjfVZ3xj",
		"1JNonvXRyZvZ4ZJ9PE8voyo67UQN1TpoGy",
		"1JrzSx8a9FVHHCkUFLB2CHULpbz4dTz5Ap",
	}

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
	return btca, addresses
}

func testNewEthAddrManager(t *testing.T, db *bolt.DB, log *logrus.Logger) (*Addrs, []string) {
	addresses := []string{
		"0x12bc2e62a27f8940c373ef1edef7b615aeb045f3",
		"0x3e0081aa902a21ff8db61b29c05889a3d1b34f45",
		"0x50e0c87ef74079650ae6cd4ee895f8e1b02714cf",
	}

	etha, err := NewAddrs(log, db, addresses, "test_bucket_eth")
	require.NoError(t, err)

	addrMap := make(map[string]struct{}, len(etha.addresses))
	for _, a := range etha.addresses {
		addrMap[a] = struct{}{}
	}

	for _, addr := range addresses {
		_, ok := addrMap[addr]
		require.True(t, ok)
	}
	return etha, addresses
}

func TestNewBtcAddrs(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)
	testNewBtcAddrManager(t, db, log)
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

	log, _ = testutil.NewLogger(t)
	btca1, err := NewAddrs(log, db, addresses, "test_bucket")
	require.NoError(t, err)

	for _, a := range btca1.addresses {
		require.NotEqual(t, a, addr)
	}

	used, err = btca1.used.IsUsed(addr)
	require.NoError(t, err)
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

func TestNewEthAddrs(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()
	log, _ := testutil.NewLogger(t)
	testNewEthAddrManager(t, db, log)
}

func TestNewEthAddress(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	addresses := []string{
		"0xc0a51efd9c319dd60d93105ab317eb362017ecb9",
		"0x3f9f942b8bd4f69432c053eef77cd84fd46b8d76",
		"0x5405f65a71342609249bb347505a4029c85ee88b",
		"0x01db29b6d512902aa82571267609f14187aa8aa8",
		"0x01db29b6d512902aa82571267609f14187aa8aa8",
	}

	log, _ := testutil.NewLogger(t)
	etha, err := NewAddrs(log, db, addresses, "test_bucket_eth")
	require.NoError(t, err)

	addr, err := etha.NewAddress()
	require.NoError(t, err)

	addrMap := make(map[string]struct{})
	for _, a := range etha.addresses {
		addrMap[a] = struct{}{}
	}

	// check if the addr still in the address pool
	_, ok := addrMap[addr]
	require.False(t, ok)

	// check if the addr is in used storage
	used, err := etha.used.IsUsed(addr)
	require.NoError(t, err)
	require.True(t, used)

	log, _ = testutil.NewLogger(t)
	etha1, err := NewAddrs(log, db, addresses, "test_bucket_eth")
	require.NoError(t, err)

	for _, a := range etha1.addresses {
		require.NotEqual(t, a, addr)
	}

	used, err = etha1.used.IsUsed(addr)
	require.NoError(t, err)
	require.True(t, used)

	// run out all addresses
	for i := 0; i < 3; i++ {
		_, err = etha1.NewAddress()
		require.NoError(t, err)
	}

	_, err = etha1.NewAddress()
	require.Error(t, err)
	require.Equal(t, ErrDepositAddressEmpty, err)
}

func TestAddrManager(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()
	log, _ := testutil.NewLogger(t)

	//create AddrGenertor
	btcGen, btcAddresses := testNewBtcAddrManager(t, db, log)
	ethGen, ethAddresses := testNewEthAddrManager(t, db, log)

	typeB := "TOKENB"
	typeE := "TOKENE"

	addrManager := NewAddrManager()
	//add generator to addrManager
	err := addrManager.PushGenerator(btcGen, typeB)
	require.NoError(t, err)
	err = addrManager.PushGenerator(ethGen, typeE)
	require.NoError(t, err)

	addrMap := make(map[string]struct{})
	for _, a := range btcAddresses {
		addrMap[a] = struct{}{}
	}
	// run out all addresses of typeB
	for i := 0; i < len(btcAddresses); i++ {
		addr, err := addrManager.NewAddress(typeB)
		require.NoError(t, err)
		//the addr still in the address pool
		_, ok := addrMap[addr]
		require.True(t, ok)
	}
	//the address pool of typeB is empty
	_, err = addrManager.NewAddress(typeB)
	require.Equal(t, ErrDepositAddressEmpty, err)

	//set typeE address into map
	addrMap = make(map[string]struct{})
	for _, a := range ethAddresses {
		addrMap[a] = struct{}{}
	}

	// run out all addresses of typeE
	for i := 0; i < len(ethAddresses); i++ {
		addr, err := addrManager.NewAddress(typeE)
		require.NoError(t, err)
		// check if the addr still in the address pool
		_, ok := addrMap[addr]
		require.True(t, ok)
	}
	_, err = addrManager.NewAddress(typeE)
	require.Equal(t, ErrDepositAddressEmpty, err)

	//check not exists cointype
	_, err = addrManager.NewAddress("OTHERTYPE")
	require.Equal(t, ErrCoinTypeNotExists, err)
}
