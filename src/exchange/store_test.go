package exchange

import (
	"testing"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/scanner"
	"github.com/skycoin/teller/src/util/dbutil"
	"github.com/skycoin/teller/src/util/testutil"
)

type MockStore struct {
	mock.Mock
}

func (m *MockStore) GetBindAddress(btcAddr string) (string, error) {
	args := m.Called(btcAddr)
	return args.String(0), args.Error(1)
}

func (m *MockStore) GetBindAddressTx(tx *bolt.Tx, btcAddr string) (string, error) {
	args := m.Called(tx, btcAddr)
	return args.String(0), args.Error(1)
}

func (m *MockStore) BindAddress(skyAddr, btcAddr string) error {
	args := m.Called(skyAddr, btcAddr)
	return args.Error(0)
}

func (m *MockStore) AddDepositInfo(di DepositInfo) error {
	args := m.Called(di)
	return args.Error(0)
}

func (m *MockStore) AddDepositInfoTx(tx *bolt.Tx, di DepositInfo) error {
	args := m.Called(tx, di)
	return args.Error(0)
}

func (m *MockStore) GetOrCreateDepositInfo(dv scanner.Deposit, rate int64) (DepositInfo, error) {
	args := m.Called(dv, rate)
	return args.Get(0).(DepositInfo), args.Error(1)
}

func (m *MockStore) GetDepositInfo(btcTx string) (DepositInfo, error) {
	args := m.Called(btcTx)
	return args.Get(0).(DepositInfo), args.Error(1)
}

func (m *MockStore) GetDepositInfoTx(tx *bolt.Tx, btcTx string) (DepositInfo, error) {
	args := m.Called(tx, btcTx)
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

func (m *MockStore) GetDepositInfoOfSkyAddress(skyAddr string) ([]DepositInfo, error) {
	args := m.Called(skyAddr)

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

func (m *MockStore) GetSkyBindBtcAddresses(skyAddr string) ([]string, error) {
	args := m.Called(skyAddr)

	btcAddrs := args.Get(0)
	if btcAddrs == nil {
		return nil, args.Error(1)
	}

	return btcAddrs.([]string), args.Error(1)
}

func (m *MockStore) GetSkyBindBtcAddressesTx(tx *bolt.Tx, skyAddr string) ([]string, error) {
	args := m.Called(tx, skyAddr)
	addrs := args.Get(0)

	if addrs == nil {
		return nil, args.Error(1)
	}

	return addrs.([]string), args.Error(1)
}

func newTestStore(t *testing.T) (*Store, func()) {
	db, shutdown := testutil.PrepareDB(t)

	log, _ := testutil.NewLogger(t)
	s, err := NewStore(log, db)
	require.NoError(t, err)

	return s, shutdown
}

func TestNewStore(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	// check the buckets
	err := s.db.View(func(tx *bolt.Tx) error {
		require.NotNil(t, tx.Bucket(exchangeMetaBkt))
		require.NotNil(t, tx.Bucket(depositInfoBkt))
		require.NotNil(t, tx.Bucket(bindAddressBkt))
		require.NotNil(t, tx.Bucket(skyDepositSeqsIndexBkt))
		require.NotNil(t, tx.Bucket(btcTxsBkt))
		return nil
	})
	require.NoError(t, err)
}

func TestAddDepositInfo(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	err := s.AddDepositInfo(DepositInfo{
		BtcTx:      "btx1:2",
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr1",
	})
	require.NoError(t, err)

	// check in db
	err = s.db.View(func(tx *bolt.Tx) error {
		var dpi DepositInfo
		err := dbutil.GetBucketObject(tx, depositInfoBkt, "btx1:2", &dpi)
		require.NoError(t, err)
		if err != nil {
			return err
		}

		require.NotEmpty(t, dpi.UpdatedAt)

		var txns []string
		err = dbutil.GetBucketObject(tx, btcTxsBkt, "btcaddr1", &txns)
		require.NoError(t, err)
		if err != nil {
			return err
		}

		require.Equal(t, "btx1:2", txns[0])

		return nil
	})
	require.NoError(t, err)

	err = s.AddDepositInfo(DepositInfo{
		BtcTx:      "btx2:2",
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr2",
	})
	require.NoError(t, err)

	err = s.db.View(func(tx *bolt.Tx) error {
		var dpi DepositInfo
		err := dbutil.GetBucketObject(tx, depositInfoBkt, "btx1:2", &dpi)
		require.NoError(t, err)
		if err != nil {
			return err
		}

		require.NotEmpty(t, dpi.UpdatedAt)

		return nil
	})
	require.NoError(t, err)

	// check invalid deposit info
	err = s.AddDepositInfo(DepositInfo{})
	require.Error(t, err)
}

func TestBindAddress(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	err := s.BindAddress("sa1", "ba1")
	require.NoError(t, err)

	// check bucekt
	err = s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bindAddressBkt)
		require.NotNil(t, bkt)
		v := bkt.Get([]byte("ba1"))
		require.Equal(t, "sa1", string(v))

		var addrs []string
		err := dbutil.GetBucketObject(tx, skyDepositSeqsIndexBkt, "sa1", &addrs)
		require.NoError(t, err)
		require.Equal(t, "ba1", addrs[0])
		return nil
	})
	require.NoError(t, err)
}

func TestGetBindAddress(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	// init the bind address bucket
	err := s.BindAddress("skyaddr1", "btcaddr1")
	require.NoError(t, err)
	err = s.BindAddress("skyaddr2", "btcaddr2")
	require.NoError(t, err)
	err = s.BindAddress("skyaddr2", "btcaddr3")
	require.NoError(t, err)

	var testCases = []struct {
		name          string
		btcAddr       string
		expectSkyAddr string
		ok            bool
		err           error
	}{
		{
			"get btcaddr1",
			"btcaddr1",
			"skyaddr1",
			true,
			nil,
		},
		{
			"get btcaddr2",
			"btcaddr2",
			"skyaddr2",
			true,
			nil,
		},
		{
			"get btcaddr3",
			"btcaddr3",
			"skyaddr2",
			true,
			nil,
		},
		{
			"get addr not exist",
			"btcaddr4",
			"",
			false,
			nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			addr, err := s.GetBindAddress(tc.btcAddr)
			require.NoError(t, err)
			if tc.ok {
				require.Equal(t, tc.expectSkyAddr, addr)
			} else {
				require.Empty(t, addr)
			}
		})
	}
}

func TestGetDepositInfo(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	err := s.AddDepositInfo(DepositInfo{
		BtcTx:      "btx1:1",
		BtcAddress: "btcaddr1",
		SkyAddress: "skyaddr1",
		Status:     StatusDone,
	})
	require.NoError(t, err)

	dpi, err := s.GetDepositInfo("btx1:1")
	require.NoError(t, err)
	require.Equal(t, "btcaddr1", dpi.BtcAddress)
	require.Equal(t, "skyaddr1", dpi.SkyAddress)
	require.Equal(t, StatusDone, dpi.Status)
	require.NotEmpty(t, dpi.UpdatedAt)
}

func TestUpdateDepositInfo(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	err := s.AddDepositInfo(DepositInfo{
		BtcTx:      "btx1:1",
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr1",
	})
	require.NoError(t, err)

	err = s.AddDepositInfo(DepositInfo{
		BtcTx:      "btx2:1",
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr2",
	})
	require.NoError(t, err)

	err = s.db.View(func(tx *bolt.Tx) error {
		var dpi1 DepositInfo
		err := dbutil.GetBucketObject(tx, depositInfoBkt, "btx1:1", &dpi1)
		require.NoError(t, err)
		if err != nil {
			return err
		}

		require.Equal(t, dpi1.Status, StatusWaitDeposit)

		var dpi2 DepositInfo
		err = dbutil.GetBucketObject(tx, depositInfoBkt, "btx2:1", &dpi2)
		require.NoError(t, err)
		if err != nil {
			return err
		}

		require.Equal(t, dpi2.Status, StatusWaitDeposit)

		return nil
	})

	require.NoError(t, err)

	dpi, err := s.UpdateDepositInfo("btx1:1", func(dpi DepositInfo) DepositInfo {
		dpi.Status = StatusWaitSend
		dpi.Txid = "121212"
		return dpi
	})

	require.NoError(t, err)
	require.Equal(t, dpi.Txid, "121212")
	require.Equal(t, dpi.Status, StatusWaitSend)

	err = s.db.View(func(tx *bolt.Tx) error {
		var dpi1 DepositInfo
		err := dbutil.GetBucketObject(tx, depositInfoBkt, "btx1:1", &dpi1)
		require.NoError(t, err)
		if err != nil {
			return err
		}

		// check updated value
		require.Equal(t, dpi1.Status, StatusWaitSend)
		require.Equal(t, "121212", dpi1.Txid)

		return nil
	})

	require.NoError(t, err)

	// TODO: test no exist deposit info
}

func TestGetDepositInfoOfSkyAddr(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	s.BindAddress("skyaddr1", "btcaddr1")

	dpis, err := s.GetDepositInfoOfSkyAddress("skyaddr1")
	require.NoError(t, err)
	require.Len(t, dpis, 1)
	require.Equal(t, dpis[0].BtcAddress, "btcaddr1")

	s.BindAddress("skyaddr1", "btcaddr2")

	dpis, err = s.GetDepositInfoOfSkyAddress("skyaddr1")
	require.NoError(t, err)
	require.Len(t, dpis, 2)
	require.Equal(t, dpis[0].BtcAddress, "btcaddr1")
	require.Equal(t, dpis[1].BtcAddress, "btcaddr2")
}

func TestGetDepositInfoArray(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	dpis := []DepositInfo{
		{
			BtcTx:      "t1:1",
			BtcAddress: "b1",
			SkyAddress: "s1",
			Status:     StatusWaitDeposit,
		},
		{
			BtcTx:      "t2:1",
			BtcAddress: "b2",
			SkyAddress: "s2",
			Status:     StatusWaitSend,
		},
	}

	for _, dpi := range dpis {
		err := s.AddDepositInfo(dpi)
		require.NoError(t, err)
	}

	ds, err := s.GetDepositInfoArray(func(dpi DepositInfo) bool {
		return dpi.Status == StatusWaitSend
	})

	require.NoError(t, err)

	require.Len(t, ds, 1)
	require.Equal(t, dpis[1].Status, ds[0].Status)
	require.Equal(t, dpis[1].BtcAddress, ds[0].BtcAddress)
	require.Equal(t, dpis[1].SkyAddress, ds[0].SkyAddress)

	ds1, err := s.GetDepositInfoArray(func(dpi DepositInfo) bool {
		return dpi.Status == StatusWaitDeposit
	})

	require.NoError(t, err)

	require.Len(t, ds1, 1)
	require.Equal(t, dpis[0].Status, ds1[0].Status)
	require.Equal(t, dpis[0].BtcAddress, ds1[0].BtcAddress)
	require.Equal(t, dpis[0].SkyAddress, ds1[0].SkyAddress)
}

func TestIsValidBtcTx(t *testing.T) {
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

func TestGetOrCreateDepositInfoAlreadyExists(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	di := DepositInfo{
		Status:     StatusWaitSend,
		BtcAddress: "foo-btc-addr",
		BtcTx:      "foo-tx:1",
		SkyBtcRate: testSkyBtcRate,
		Deposit: scanner.Deposit{
			Address: "foo-btc-addr",
			Value:   1e6,
			Height:  20,
			Tx:      "foo-tx",
			N:       1,
		},
	}

	err := s.AddDepositInfo(di)
	require.NoError(t, err)

	// Check the saved deposit info
	foundDi, err := s.GetDepositInfo(di.BtcTx)
	require.NoError(t, err)
	// Seq and UpdatedAt should be set by AddDepositInfo
	require.Equal(t, uint64(1), foundDi.Seq)
	require.NotEmpty(t, foundDi.UpdatedAt)

	// Other fields should be unchanged
	di.Seq = foundDi.Seq
	di.UpdatedAt = foundDi.UpdatedAt
	require.Equal(t, di, foundDi)

	// GetOrCreateDepositInfo, deposit info exists
	dv := scanner.Deposit{
		Address: di.Deposit.Address + "-2",
		Value:   di.Deposit.Value * 2,
		Height:  di.Deposit.Height + 1,
		Tx:      di.Deposit.Tx,
		N:       di.Deposit.N,
	}
	require.Equal(t, di.Deposit.TxN(), dv.TxN())
	existsDi, err := s.GetOrCreateDepositInfo(dv, di.SkyBtcRate*2)

	// di.Deposit won't be changed
	require.Equal(t, di, existsDi)
}

func TestGetOrCreateDepositInfoNoBoundSkyAddr(t *testing.T) {
	s, shutdown := newTestStore(t)
	defer shutdown()

	var rate int64 = 100
	dv := scanner.Deposit{
		Address: "foo-btc-addr",
	}

	_, err := s.GetOrCreateDepositInfo(dv, rate)
	require.Error(t, err)
	require.Equal(t, err, ErrNoBoundAddress)
}
