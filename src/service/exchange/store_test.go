package exchange

import (
	"testing"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/dbutil"
	"github.com/skycoin/teller/src/service/testutil"
)

func TestNewStore(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	_, err := newStore(db)
	require.NoError(t, err)

	// check the buckets
	db.View(func(tx *bolt.Tx) error {
		require.NotNil(t, tx.Bucket(exchangeMetaBkt))
		require.NotNil(t, tx.Bucket(depositInfoBkt))
		require.NotNil(t, tx.Bucket(bindAddressBkt))
		require.NotNil(t, tx.Bucket(skyDepositSeqsIndexBkt))
		require.NotNil(t, tx.Bucket(btcTxsBkt))
		return nil
	})
}

func TestAddDepositInfo(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.NoError(t, err)

	err = s.AddDepositInfo(DepositInfo{
		BtcTx:      "btx1:2",
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr1",
	})
	require.NoError(t, err)

	// check in db
	err = db.View(func(tx *bolt.Tx) error {
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

	err = db.View(func(tx *bolt.Tx) error {
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
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()
	s, err := newStore(db)
	require.NoError(t, err)

	err = s.BindAddress("sa1", "ba1")
	require.NoError(t, err)

	// check bucekt
	db.View(func(tx *bolt.Tx) error {
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
}

func TestGetBindAddress(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.NoError(t, err)

	// init the bind address bucket
	err = s.BindAddress("skyaddr1", "btcaddr1")
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
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.NoError(t, err)

	err = s.AddDepositInfo(DepositInfo{
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
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.NoError(t, err)

	err = s.AddDepositInfo(DepositInfo{
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

	err = db.View(func(tx *bolt.Tx) error {
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

	err = s.UpdateDepositInfo("btx1:1", func(dpi DepositInfo) DepositInfo {
		dpi.Status = StatusWaitSend
		dpi.Txid = "121212"
		return dpi
	})

	require.NoError(t, err)

	err = db.View(func(tx *bolt.Tx) error {
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
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.NoError(t, err)

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
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.NoError(t, err)

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
