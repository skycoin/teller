package exchange

import (
	"testing"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/MDLlife/teller/src/config"
	"github.com/MDLlife/teller/src/scanner"
	"github.com/MDLlife/teller/src/util/dbutil"
	"github.com/MDLlife/teller/src/util/testutil"
)

type MockStore struct {
	mock.Mock
}

func (m *MockStore) GetBindAddress(btcAddr, coinType string) (*BoundAddress, error) {
	args := m.Called(btcAddr, coinType)

	ba := args.Get(0)
	if ba == nil {
		return nil, args.Error(1)
	}

	return ba.(*BoundAddress), args.Error(1)
}

func (m *MockStore) BindAddress(mdlAddr, btcAddr, coinType, buyMethod string) (*BoundAddress, error) {
	args := m.Called(mdlAddr, btcAddr, coinType, buyMethod)

	ba := args.Get(0)
	if ba == nil {
		return nil, args.Error(1)
	}

	return ba.(*BoundAddress), args.Error(1)
}

func (m *MockStore) GetOrCreateDepositInfo(dv scanner.Deposit, rate string) (DepositInfo, error) {
	args := m.Called(dv, rate)
	return args.Get(0).(DepositInfo), args.Error(1)
}

func (m *MockStore) GetDepositInfoArray(filt DepositFilter) ([]DepositInfo, error) {
	args := m.Called(filt)

	dis := args.Get(0)
	if dis == nil {
		return nil, args.Error(1)
	}

	return dis.([]DepositInfo), args.Error(1)
}

func (m *MockStore) GetDepositInfoOfMDLAddress(mdlAddr string) ([]DepositInfo, error) {
	args := m.Called(mdlAddr)

	dis := args.Get(0)
	if dis == nil {
		return nil, args.Error(1)
	}

	return dis.([]DepositInfo), args.Error(1)
}

func (m *MockStore) UpdateDepositInfo(btcTx string, f func(DepositInfo) DepositInfo) (DepositInfo, error) {
	args := m.Called(btcTx, f)
	return args.Get(0).(DepositInfo), args.Error(1)
}

func (m *MockStore) UpdateDepositInfoCallback(btcTx string, f func(DepositInfo) DepositInfo, callback func(DepositInfo) error) (DepositInfo, error) {
	args := m.Called(btcTx, f, callback)
	return args.Get(0).(DepositInfo), args.Error(1)
}

func (m *MockStore) GetMDLBindAddresses(mdlAddr string) ([]BoundAddress, error) {
	args := m.Called(mdlAddr)

	btcAddrs := args.Get(0)
	if btcAddrs == nil {
		return nil, args.Error(1)
	}

	return btcAddrs.([]BoundAddress), args.Error(1)
}

func (m *MockStore) GetDepositStats() (int64, int64, error) {
	args := m.Called()
	return args.Get(0).(int64), args.Get(1).(int64), args.Error(2)
}

func newTestStore(t *testing.T) (*Store, func()) {
	db, shutdown := testutil.PrepareDB(t)

	log, _ := testutil.NewLogger(t)
	s, err := NewStore(log, db)
	require.NoError(t, err)

	return s, shutdown
}

func TestStoreNewStore(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	// check the buckets
	err := s.db.View(func(tx *bolt.Tx) error {
		require.NotNil(t, tx.Bucket(ExchangeMetaBkt))
		require.NotNil(t, tx.Bucket(DepositInfoBkt))
		require.NotNil(t, tx.Bucket(MustGetBindAddressBkt(scanner.CoinTypeBTC)))
		require.NotNil(t, tx.Bucket(MustGetBindAddressBkt(scanner.CoinTypeETH)))
		require.NotNil(t, tx.Bucket(MustGetBindAddressBkt(scanner.CoinTypeSKY)))
		require.NotNil(t, tx.Bucket(MustGetBindAddressBkt(scanner.CoinTypeWAVES)))
		require.NotNil(t, tx.Bucket(MDLDepositSeqsIndexBkt))
		require.NotNil(t, tx.Bucket(BtcTxsBkt))
		return nil
	})
	require.NoError(t, err)
}

func TestStoreAddDepositInfo(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	di, err := s.addDepositInfo(DepositInfo{
		DepositID:      "btx1:2",
		MDLAddress:     "mdladdr1",
		DepositAddress: "btcaddr1",
		DepositValue:   1e6,
		ConversionRate: testMDLBtcRate,
		Status:         StatusWaitSend,
		BuyMethod:      config.BuyMethodDirect,
	})
	require.NoError(t, err)
	require.Equal(t, di.Seq, uint64(1))
	require.NotEmpty(t, di.UpdatedAt)

	// check in db
	err = s.db.View(func(tx *bolt.Tx) error {
		var dpi DepositInfo
		err := dbutil.GetBucketObject(tx, DepositInfoBkt, "btx1:2", &dpi)
		require.NoError(t, err)
		if err != nil {
			return err
		}

		require.NotEmpty(t, dpi.UpdatedAt)

		var txns []string
		err = dbutil.GetBucketObject(tx, BtcTxsBkt, "btcaddr1", &txns)
		require.NoError(t, err)
		if err != nil {
			return err
		}

		require.Equal(t, "btx1:2", txns[0])

		return nil
	})
	require.NoError(t, err)

	_, err = s.addDepositInfo(DepositInfo{
		DepositID:      "btx2:2",
		MDLAddress:     "mdladdr1",
		DepositAddress: "btcaddr2",
		DepositValue:   1e6,
		ConversionRate: testMDLBtcRate,
		Status:         StatusWaitSend,
		BuyMethod:      config.BuyMethodDirect,
	})
	require.NoError(t, err)

	err = s.db.View(func(tx *bolt.Tx) error {
		var dpi DepositInfo
		err := dbutil.GetBucketObject(tx, DepositInfoBkt, "btx1:2", &dpi)
		require.NoError(t, err)
		if err != nil {
			return err
		}

		require.NotEmpty(t, dpi.UpdatedAt)

		return nil
	})
	require.NoError(t, err)

	// check invalid deposit info
	_, err = s.addDepositInfo(DepositInfo{})
	require.Error(t, err)
}

func mustBindAddress(t *testing.T, s Storer, mdlAddr, addr string) {
	boundAddr, err := s.BindAddress(mdlAddr, addr, scanner.CoinTypeBTC, config.BuyMethodDirect)
	require.NoError(t, err)
	require.NotNil(t, boundAddr)
	require.Equal(t, mdlAddr, boundAddr.MDLAddress)
	require.Equal(t, addr, boundAddr.Address)
	require.Equal(t, scanner.CoinTypeBTC, boundAddr.CoinType)
	require.Equal(t, config.BuyMethodDirect, boundAddr.BuyMethod)
}

func mustBindAddressSky(t *testing.T, s Storer, mdlAddr, addr string) {
	boundAddr, err := s.BindAddress(mdlAddr, addr, scanner.CoinTypeSKY, config.BuyMethodDirect)
	require.NoError(t, err)
	require.NotNil(t, boundAddr)
	require.Equal(t, mdlAddr, boundAddr.MDLAddress)
	require.Equal(t, addr, boundAddr.Address)
	require.Equal(t, scanner.CoinTypeSKY, boundAddr.CoinType)
	require.Equal(t, config.BuyMethodDirect, boundAddr.BuyMethod)
}

func mustBindAddressWaves(t *testing.T, s Storer, mdlAddr, addr string) {
	boundAddr, err := s.BindAddress(mdlAddr, addr, scanner.CoinTypeWAVES, config.BuyMethodDirect)
	require.NoError(t, err)
	require.NotNil(t, boundAddr)
	require.Equal(t, mdlAddr, boundAddr.MDLAddress)
	require.Equal(t, addr, boundAddr.Address)
	require.Equal(t, scanner.CoinTypeWAVES, boundAddr.CoinType)
	require.Equal(t, config.BuyMethodDirect, boundAddr.BuyMethod)
}

func TestStoreBindAddress(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	mustBindAddress(t, s, "sa1", "ba1")

	// check bucket
	err := s.db.View(func(tx *bolt.Tx) error {
		bktName := MustGetBindAddressBkt(scanner.CoinTypeBTC)
		var ba BoundAddress
		err := dbutil.GetBucketObject(tx, bktName, "ba1", &ba)
		require.NoError(t, err)
		require.Equal(t, BoundAddress{
			MDLAddress: "sa1",
			Address:    "ba1",
			CoinType:   scanner.CoinTypeBTC,
			BuyMethod:  config.BuyMethodDirect,
		}, ba)

		var addrs []BoundAddress
		err = dbutil.GetBucketObject(tx, MDLDepositSeqsIndexBkt, "sa1", &addrs)
		require.NoError(t, err)
		require.Equal(t, BoundAddress{
			MDLAddress: "sa1",
			Address:    "ba1",
			CoinType:   scanner.CoinTypeBTC,
			BuyMethod:  config.BuyMethodDirect,
		}, addrs[0])

		return nil
	})
	require.NoError(t, err)

	// A mdl address can have multiple addresses bound to it
	mustBindAddress(t, s, "sa1", "ba2")
}

func TestStoreSkyBindAddress(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	mustBindAddressSky(t, s, "sa12", "ba12")

	// check bucket
	err := s.db.View(func(tx *bolt.Tx) error {
		bktName := MustGetBindAddressBkt(scanner.CoinTypeSKY)
		var ba BoundAddress
		err := dbutil.GetBucketObject(tx, bktName, "ba12", &ba)
		require.NoError(t, err)
		require.Equal(t, BoundAddress{
			MDLAddress: "sa12",
			Address:    "ba12",
			CoinType:   scanner.CoinTypeSKY,
			BuyMethod:  config.BuyMethodDirect,
		}, ba)

		var addrs []BoundAddress
		err = dbutil.GetBucketObject(tx, MDLDepositSeqsIndexBkt, "sa12", &addrs)
		require.NoError(t, err)
		require.Equal(t, BoundAddress{
			MDLAddress: "sa12",
			Address:    "ba12",
			CoinType:   scanner.CoinTypeSKY,
			BuyMethod:  config.BuyMethodDirect,
		}, addrs[0])

		return nil
	})
	require.NoError(t, err)

	// A mdl address can have multiple addresses bound to it
	mustBindAddressSky(t, s, "sa12", "ba22")
}

func TestStoreWavesBindAddress(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	mustBindAddressWaves(t, s, "sa12", "ba12")

	// check bucket
	err := s.db.View(func(tx *bolt.Tx) error {
		bktName := MustGetBindAddressBkt(scanner.CoinTypeWAVES)
		var ba BoundAddress
		err := dbutil.GetBucketObject(tx, bktName, "ba12", &ba)
		require.NoError(t, err)
		require.Equal(t, BoundAddress{
			MDLAddress: "sa12",
			Address:    "ba12",
			CoinType:   scanner.CoinTypeWAVES,
			BuyMethod:  config.BuyMethodDirect,
		}, ba)

		var addrs []BoundAddress
		err = dbutil.GetBucketObject(tx, MDLDepositSeqsIndexBkt, "sa12", &addrs)
		require.NoError(t, err)
		require.Equal(t, BoundAddress{
			MDLAddress: "sa12",
			Address:    "ba12",
			CoinType:   scanner.CoinTypeWAVES,
			BuyMethod:  config.BuyMethodDirect,
		}, addrs[0])

		return nil
	})
	require.NoError(t, err)

	// A mdl address can have multiple addresses bound to it
	mustBindAddressSky(t, s, "sa12", "ba22")
}

func TestStoreBindAddressTwiceFails(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	mustBindAddress(t, s, "a", "b")

	boundAddr, err := s.BindAddress("a", "b", scanner.CoinTypeBTC, config.BuyMethodDirect)
	require.Error(t, err)
	require.Equal(t, ErrAddressAlreadyBound, err)
	require.Nil(t, boundAddr)

	boundAddr, err = s.BindAddress("c", "b", scanner.CoinTypeBTC, config.BuyMethodDirect)
	require.Error(t, err)
	require.Equal(t, ErrAddressAlreadyBound, err)
	require.Nil(t, boundAddr)
}

func TestStoreGetBindAddress(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	var testCases = []struct {
		name          string
		coinAddr      string
		expectMDLAddr string
		ok            bool
		err           error
		coinType      string
	}{
		{
			"get btcaddr1",
			"btcaddr1",
			"mdladdr1",
			true,
			nil,
			scanner.CoinTypeBTC,
		},
		{
			"get btcaddr2",
			"btcaddr2",
			"mdladdr2",
			true,
			nil,
			scanner.CoinTypeBTC,
		},
		{
			"get btcaddr3",
			"btcaddr3",
			"mdladdr2",
			true,
			nil,
			scanner.CoinTypeBTC,
		},
		{
			"get addr not exist",
			"btcaddr4",
			"",
			false,
			nil,
			scanner.CoinTypeBTC,
		},
		{
			"get skyaddr1",
			"skyaddr1",
			"mdladdr12",
			true,
			nil,
			scanner.CoinTypeSKY,
		},
		{
			"get skyaddr2",
			"skyaddr2",
			"mdladdr22",
			true,
			nil,
			scanner.CoinTypeSKY,
		},
		{
			"get skyaddr3",
			"skyaddr3",
			"mdladdr22",
			true,
			nil,
			scanner.CoinTypeSKY,
		},
		{
			"get skyaddr not exist",
			"skyaddr42",
			"",
			false,
			nil,
			scanner.CoinTypeSKY,
		},
		{
			"get wavesaddr1",
			"wavesaddr1",
			"mdladdr123",
			true,
			nil,
			scanner.CoinTypeWAVES,
		},
		{
			"get wavesaddr2",
			"wavesaddr2",
			"mdladdr223",
			true,
			nil,
			scanner.CoinTypeWAVES,
		},
		{
			"get wavesaddr3",
			"wavesaddr3",
			"mdladdr223",
			true,
			nil,
			scanner.CoinTypeWAVES,
		},
		{
			"get wavesaddr not exist",
			"skyaddr423",
			"",
			false,
			nil,
			scanner.CoinTypeWAVES,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// init the bind address bucket
			if tc.expectMDLAddr != "" {
				if tc.coinType == scanner.CoinTypeSKY {
					mustBindAddressSky(t, s, tc.expectMDLAddr, tc.coinAddr)
				} else if tc.coinType == scanner.CoinTypeWAVES {
					mustBindAddressWaves(t, s, tc.expectMDLAddr, tc.coinAddr)
				} else {
					mustBindAddress(t, s, tc.expectMDLAddr, tc.coinAddr)
				}
			}

			addr, err := s.GetBindAddress(tc.coinAddr, tc.coinType)
			require.NoError(t, err)
			if tc.ok {
				require.NotNil(t, addr)
				require.Equal(t, BoundAddress{
					MDLAddress: tc.expectMDLAddr,
					Address:    tc.coinAddr,
					CoinType:   tc.coinType,
					BuyMethod:  config.BuyMethodDirect,
				}, *addr)
			} else {
				require.Nil(t, addr)
			}
		})
	}
}

func TestStoreGetDepositInfo(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	_, err := s.addDepositInfo(DepositInfo{
		DepositID:      "btx1:1",
		DepositAddress: "btcaddr1",
		MDLAddress:     "mdladdr1",
		DepositValue:   1e6,
		Txid:           "txid-1",
		ConversionRate: testMDLBtcRate,
		MDLSent:        100e8,
		Status:         StatusDone,
		BuyMethod:      config.BuyMethodDirect,
	})
	require.NoError(t, err)

	dpi, err := s.getDepositInfo("btx1:1")
	require.NoError(t, err)
	require.Equal(t, "btcaddr1", dpi.DepositAddress)
	require.Equal(t, "mdladdr1", dpi.MDLAddress)
	require.Equal(t, StatusDone, dpi.Status)
	require.NotEmpty(t, dpi.UpdatedAt)
}

func TestStoreUpdateDepositInfo(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	_, err := s.addDepositInfo(DepositInfo{
		DepositID:      "btx1:1",
		MDLAddress:     "mdladdr1",
		DepositAddress: "btcaddr1",
		DepositValue:   1e6,
		ConversionRate: testMDLBtcRate,
		Status:         StatusWaitSend,
		BuyMethod:      config.BuyMethodDirect,
	})
	require.NoError(t, err)

	_, err = s.addDepositInfo(DepositInfo{
		DepositID:      "btx2:1",
		MDLAddress:     "mdladdr1",
		DepositAddress: "btcaddr2",
		DepositValue:   1e6,
		ConversionRate: testMDLBtcRate,
		Status:         StatusWaitSend,
		BuyMethod:      config.BuyMethodDirect,
	})
	require.NoError(t, err)

	err = s.db.View(func(tx *bolt.Tx) error {
		var dpi1 DepositInfo
		err := dbutil.GetBucketObject(tx, DepositInfoBkt, "btx1:1", &dpi1)
		require.NoError(t, err)
		if err != nil {
			return err
		}

		require.Equal(t, dpi1.Status, StatusWaitSend)

		var dpi2 DepositInfo
		err = dbutil.GetBucketObject(tx, DepositInfoBkt, "btx2:1", &dpi2)
		require.NoError(t, err)
		if err != nil {
			return err
		}

		require.Equal(t, dpi2.Status, StatusWaitSend)

		return nil
	})

	require.NoError(t, err)

	dpi, err := s.UpdateDepositInfo("btx1:1", func(dpi DepositInfo) DepositInfo {
		dpi.Status = StatusWaitConfirm
		dpi.Txid = "121212"
		return dpi
	})

	require.NoError(t, err)
	require.Equal(t, dpi.Txid, "121212")
	require.Equal(t, dpi.Status, StatusWaitConfirm)

	err = s.db.View(func(tx *bolt.Tx) error {
		var dpi1 DepositInfo
		err := dbutil.GetBucketObject(tx, DepositInfoBkt, "btx1:1", &dpi1)
		require.NoError(t, err)
		if err != nil {
			return err
		}

		// check updated value
		require.Equal(t, dpi1.Status, StatusWaitConfirm)
		require.Equal(t, "121212", dpi1.Txid)

		return nil
	})

	require.NoError(t, err)

	// TODO: test no exist deposit info
}

func TestStoreGetDepositInfoOfMDLAddress(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	mustBindAddress(t, s, "mdladdr1", "btcaddr1")

	dpis, err := s.GetDepositInfoOfMDLAddress("mdladdr1")
	require.NoError(t, err)
	require.Len(t, dpis, 1)
	require.Equal(t, dpis[0].DepositAddress, "btcaddr1")

	mustBindAddress(t, s, "mdladdr1", "btcaddr2")

	dpis, err = s.GetDepositInfoOfMDLAddress("mdladdr1")
	require.NoError(t, err)
	require.Len(t, dpis, 2)
	require.Equal(t, dpis[0].DepositAddress, "btcaddr1")
	require.Equal(t, dpis[1].DepositAddress, "btcaddr2")

	// Multiple txns saved
	di3 := DepositInfo{
		MDLAddress:     "mdladdr3",
		DepositAddress: "btcaddr3",
		DepositID:      "btctx:3",
		DepositValue:   100e8,
		ConversionRate: testMDLBtcRate,
		Status:         StatusWaitSend,
		BuyMethod:      config.BuyMethodDirect,
	}
	di3, err = s.addDepositInfo(di3)
	require.Equal(t, di3.Seq, uint64(1))
	require.NoError(t, err)

	mustBindAddress(t, s, "mdladdr3", "btcaddr3")
	mustBindAddress(t, s, "mdladdr3", "btcaddr4")

	di4 := DepositInfo{
		MDLAddress:     "mdladdr3",
		DepositAddress: "btcaddr4",
		DepositID:      "btctx:4",
		DepositValue:   1000e8,
		ConversionRate: testMDLBtcRate,
		Status:         StatusWaitSend,
		BuyMethod:      config.BuyMethodDirect,
	}
	di4, err = s.addDepositInfo(di4)
	require.NoError(t, err)

	dpis, err = s.GetDepositInfoOfMDLAddress("mdladdr3")
	require.NoError(t, err)
	t.Logf("%v", dpis)
	require.Len(t, dpis, 2)

	// Sequences are renumbered in the result, starting from 0
	di3.Seq = 0
	di4.Seq = 1

	require.Equal(t, di3, dpis[0])
	require.Equal(t, di4, dpis[1])
}

func TestStoreGetDepositInfoArray(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	dpis := []DepositInfo{
		{
			DepositID:      "t1:1",
			DepositAddress: "b1",
			MDLAddress:     "s1",
			DepositValue:   1e6,
			ConversionRate: testMDLBtcRate,
			Status:         StatusWaitSend,
			BuyMethod:      config.BuyMethodDirect,
		},
		{
			DepositID:      "t2:1",
			DepositAddress: "b2",
			MDLAddress:     "s2",
			DepositValue:   1e6,
			Txid:           "txid-2",
			ConversionRate: testMDLBtcRate,
			MDLSent:        100e8,
			BuyMethod:      config.BuyMethodDirect,
			Status:         StatusWaitConfirm,
		},
	}

	for _, dpi := range dpis {
		_, err := s.addDepositInfo(dpi)
		require.NoError(t, err)
	}

	ds, err := s.GetDepositInfoArray(func(dpi DepositInfo) bool {
		return dpi.Status == StatusWaitSend
	})

	require.NoError(t, err)

	require.Len(t, ds, 1)
	require.Equal(t, dpis[0].Status, ds[0].Status)
	require.Equal(t, dpis[0].DepositAddress, ds[0].DepositAddress)
	require.Equal(t, dpis[0].MDLAddress, ds[0].MDLAddress)

	ds1, err := s.GetDepositInfoArray(func(dpi DepositInfo) bool {
		return dpi.Status == StatusWaitConfirm
	})

	require.NoError(t, err)

	require.Len(t, ds1, 1)
	require.Equal(t, dpis[1].Status, ds1[0].Status)
	require.Equal(t, dpis[1].DepositAddress, ds1[0].DepositAddress)
	require.Equal(t, dpis[1].MDLAddress, ds1[0].MDLAddress)
}

func TestStoreIsValidBtcTx(t *testing.T) {
	cases := []struct {
		name  string
		valid bool
		btctx string
	}{
		{
			"empty string",
			false,
			"",
		},
		{
			"colon only",
			false,
			":",
		},
		{
			"multiple colons",
			false,
			"txid:2:2",
		},
		{
			"no txid",
			false,
			":2",
		},
		{
			"no n",
			false,
			"txid:",
		},
		{
			"n not int",
			false,
			"txid:b",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.valid, isValidBtcTx(tc.btctx))
		})
	}
}

func TestStoreGetOrCreateDepositInfoAlreadyExists(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	di := DepositInfo{
		CoinType:       scanner.CoinTypeBTC,
		Status:         StatusWaitSend,
		DepositAddress: "foo-btc-addr",
		DepositID:      "foo-tx:1",
		MDLAddress:     "foo-mdl-addr",
		DepositValue:   1e6,
		BuyMethod:      config.BuyMethodDirect,
		ConversionRate: testMDLBtcRate,
		Deposit: scanner.Deposit{
			CoinType: scanner.CoinTypeBTC,
			Address:  "foo-btc-addr",
			Value:    1e6,
			Height:   20,
			Tx:       "foo-tx",
			N:        1,
		},
	}

	_, err := s.addDepositInfo(di)
	require.NoError(t, err)

	// Check the saved deposit info
	foundDi, err := s.getDepositInfo(di.DepositID)
	require.NoError(t, err)
	// Seq and UpdatedAt should be set by addDepositInfo
	require.Equal(t, uint64(1), foundDi.Seq)
	require.NotEmpty(t, foundDi.UpdatedAt)

	// Other fields should be unchanged
	di.Seq = foundDi.Seq
	di.UpdatedAt = foundDi.UpdatedAt
	require.Equal(t, di, foundDi)

	// GetOrCreateDepositInfo, deposit info exists
	dv := scanner.Deposit{
		CoinType: scanner.CoinTypeBTC,
		Address:  di.Deposit.Address + "-2",
		Value:    di.Deposit.Value * 2,
		Height:   di.Deposit.Height + 1,
		Tx:       di.Deposit.Tx,
		N:        di.Deposit.N,
	}
	require.Equal(t, di.Deposit.ID(), dv.ID())

	differentRate := "112233"
	require.NotEqual(t, differentRate, di.ConversionRate)
	existsDi, err := s.GetOrCreateDepositInfo(dv, differentRate)
	require.NoError(t, err)

	// di.Deposit won't be changed
	require.Equal(t, di, existsDi)
}

func TestStoreSkyGetOrCreateDepositInfoAlreadyExists(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	di := DepositInfo{
		CoinType:       scanner.CoinTypeSKY,
		Status:         StatusWaitSend,
		DepositAddress: "foo-sky-addr",
		DepositID:      "foo-tx:1",
		MDLAddress:     "foo-mdl-addr",
		DepositValue:   1e6,
		BuyMethod:      config.BuyMethodDirect,
		ConversionRate: testMDLBtcRate,
		Deposit: scanner.Deposit{
			CoinType: scanner.CoinTypeSKY,
			Address:  "foo-sky-addr",
			Value:    1e6,
			Height:   20,
			Tx:       "foo-tx",
			N:        1,
		},
	}

	_, err := s.addDepositInfo(di)
	require.NoError(t, err)

	// Check the saved deposit info
	foundDi, err := s.getDepositInfo(di.DepositID)
	require.NoError(t, err)
	// Seq and UpdatedAt should be set by addDepositInfo
	require.Equal(t, uint64(1), foundDi.Seq)
	require.NotEmpty(t, foundDi.UpdatedAt)

	// Other fields should be unchanged
	di.Seq = foundDi.Seq
	di.UpdatedAt = foundDi.UpdatedAt
	require.Equal(t, di, foundDi)

	// GetOrCreateDepositInfo, deposit info exists
	dv := scanner.Deposit{
		CoinType: scanner.CoinTypeSKY,
		Address:  di.Deposit.Address + "-2",
		Value:    di.Deposit.Value * 2,
		Height:   di.Deposit.Height + 1,
		Tx:       di.Deposit.Tx,
		N:        di.Deposit.N,
	}
	require.Equal(t, di.Deposit.ID(), dv.ID())

	differentRate := "112233"
	require.NotEqual(t, differentRate, di.ConversionRate)
	existsDi, err := s.GetOrCreateDepositInfo(dv, differentRate)
	require.NoError(t, err)

	// di.Deposit won't be changed
	require.Equal(t, di, existsDi)
}

func TestStoreWavesGetOrCreateDepositInfoAlreadyExists(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	di := DepositInfo{
		CoinType:       scanner.CoinTypeWAVES,
		Status:         StatusWaitSend,
		DepositAddress: "foo-waves-addr",
		DepositID:      "foo-tx:1",
		MDLAddress:     "foo-mdl-addr",
		DepositValue:   1e6,
		BuyMethod:      config.BuyMethodDirect,
		ConversionRate: testMDLWavesRate,
		Deposit: scanner.Deposit{
			CoinType: scanner.CoinTypeWAVES,
			Address:  "foo-waves-addr",
			Value:    1e6,
			Height:   20,
			Tx:       "foo-tx",
			N:        1,
		},
	}

	_, err := s.addDepositInfo(di)
	require.NoError(t, err)

	// Check the saved deposit info
	foundDi, err := s.getDepositInfo(di.DepositID)
	require.NoError(t, err)
	// Seq and UpdatedAt should be set by addDepositInfo
	require.Equal(t, uint64(1), foundDi.Seq)
	require.NotEmpty(t, foundDi.UpdatedAt)

	// Other fields should be unchanged
	di.Seq = foundDi.Seq
	di.UpdatedAt = foundDi.UpdatedAt
	require.Equal(t, di, foundDi)

	// GetOrCreateDepositInfo, deposit info exists
	dv := scanner.Deposit{
		CoinType: scanner.CoinTypeWAVES,
		Address:  di.Deposit.Address + "-2",
		Value:    di.Deposit.Value * 2,
		Height:   di.Deposit.Height + 1,
		Tx:       di.Deposit.Tx,
		N:        di.Deposit.N,
	}
	require.Equal(t, di.Deposit.ID(), dv.ID())

	differentRate := "112233"
	require.NotEqual(t, differentRate, di.ConversionRate)
	existsDi, err := s.GetOrCreateDepositInfo(dv, differentRate)
	require.NoError(t, err)

	// di.Deposit won't be changed
	require.Equal(t, di, existsDi)
}

func TestStoreGetOrCreateDepositInfoNoBoundMDLAddr(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	dv := scanner.Deposit{
		Address:  "foo-btc-addr",
		CoinType: scanner.CoinTypeBTC,
	}

	rate := "100"
	_, err := s.GetOrCreateDepositInfo(dv, rate)
	require.Error(t, err)
	require.Equal(t, err, ErrNoBoundAddress)

	dv = scanner.Deposit{
		Address:  "foo-sky-addr",
		CoinType: scanner.CoinTypeSKY,
	}

	_, err = s.GetOrCreateDepositInfo(dv, rate)
	require.Error(t, err)
	require.Equal(t, err, ErrNoBoundAddress)

	dv = scanner.Deposit{
		Address:  "foo-waves-addr",
		CoinType: scanner.CoinTypeWAVES,
	}

	_, err = s.GetOrCreateDepositInfo(dv, rate)
	require.Error(t, err)
	require.Equal(t, err, ErrNoBoundAddress)
}

func TestStoreGetMDLBindAddresses(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	mdlAddr := "mdlAddr"
	addrs, err := s.GetMDLBindAddresses(mdlAddr)
	require.NoError(t, err)
	require.Nil(t, addrs)

	btcAddr1 := "btcaddr1"
	mustBindAddress(t, s, mdlAddr, btcAddr1)

	addrs, err = s.GetMDLBindAddresses(mdlAddr)
	require.NoError(t, err)
	require.Len(t, addrs, 1)
	require.Equal(t, addrs[0], BoundAddress{
		Address:    btcAddr1,
		MDLAddress: mdlAddr,
		BuyMethod:  config.BuyMethodDirect,
		CoinType:   scanner.CoinTypeBTC,
	})

	btcAddr2 := "btcaddr2"
	mustBindAddress(t, s, mdlAddr, btcAddr2)

	skyAddr1 := "skyaddr1"
	mustBindAddressSky(t, s, mdlAddr, skyAddr1)

	wavesAddr1 := "wavesaddr1"
	mustBindAddressWaves(t, s, mdlAddr, wavesAddr1)

	addrs, err = s.GetMDLBindAddresses(mdlAddr)
	require.NoError(t, err)
	require.Len(t, addrs, 4)
	require.Equal(t, addrs[0], BoundAddress{
		Address:    btcAddr1,
		MDLAddress: mdlAddr,
		BuyMethod:  config.BuyMethodDirect,
		CoinType:   scanner.CoinTypeBTC,
	})
	require.Equal(t, addrs[1], BoundAddress{
		Address:    btcAddr2,
		MDLAddress: mdlAddr,
		BuyMethod:  config.BuyMethodDirect,
		CoinType:   scanner.CoinTypeBTC,
	})

	require.Equal(t, addrs[2], BoundAddress{
		Address:    skyAddr1,
		MDLAddress: mdlAddr,
		BuyMethod:  config.BuyMethodDirect,
		CoinType:   scanner.CoinTypeSKY,
	})

	require.Equal(t, addrs[3], BoundAddress{
		Address:    wavesAddr1,
		MDLAddress: mdlAddr,
		BuyMethod:  config.BuyMethodDirect,
		CoinType:   scanner.CoinTypeWAVES,
	})

}
