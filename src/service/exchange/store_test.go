package exchange

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/require"
)

func setupDB(t *testing.T) (*bolt.DB, func()) {
	rand.Seed(int64(time.Now().Second()))
	f := fmt.Sprintf("%s/test%d.db", os.TempDir(), rand.Intn(1024))
	db, err := bolt.Open(f, 0700, nil)
	require.Nil(t, err)
	return db, func() {
		db.Close()
		os.Remove(f)
	}
}

func TestNewStore(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	_, err := newStore(db)
	require.Nil(t, err)

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

func TestBytesUintConvert(t *testing.T) {
	v := uint64(10)

	b := uint64ToBytes(v)
	require.Equal(t, 8, len(b))

	v2 := bytesToUint64(b)
	require.Equal(t, v, v2)
}

func TestAddDepositInfo(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	err = s.AddDepositInfo(DepositInfo{
		BtcTx:      "btx1",
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr1",
	})
	require.Nil(t, err)

	// check the deposit info cache
	dpi, ok := s.cache.depositInfo["btx1"]
	require.True(t, ok)

	require.Equal(t, "skyaddr1", dpi.SkyAddress)
	require.Equal(t, "btcaddr1", dpi.BtcAddress)
	require.Equal(t, "btx1", dpi.BtcTx)
	require.Equal(t, StatusWaitDeposit, dpi.Status)
	require.NotEmpty(t, dpi.UpdatedAt)

	// check btc tx
	txs := s.cache.btcTxs["btcaddr1"]
	require.Equal(t, "btx1", txs[0])

	// check in db
	db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(depositInfoBkt).Get([]byte("btx1"))
		require.NotNil(t, v)
		var dpi DepositInfo
		require.Nil(t, json.Unmarshal(v, &dpi))
		require.NotEmpty(t, dpi.UpdatedAt)

		v = tx.Bucket(btcTxsBkt).Get([]byte("btcaddr1"))
		require.NotNil(t, v)
		txs := []string{}
		err = json.Unmarshal(v, &txs)
		require.Nil(t, err)
		require.Equal(t, "btx1", txs[0])

		return nil
	})

	err = s.AddDepositInfo(DepositInfo{
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr2",
		BtcTx:      "btx2",
	})

	require.Nil(t, err)
	db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(depositInfoBkt).Get([]byte("btx2"))
		require.NotNil(t, v)
		var dpi DepositInfo
		require.Nil(t, json.Unmarshal(v, &dpi))
		require.NotEmpty(t, dpi.UpdatedAt)
		return nil
	})

	// check invalid deposit info
	err = s.AddDepositInfo(DepositInfo{})
	require.NotNil(t, err)
}

func TestBindAddress(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()
	s, err := newStore(db)
	require.Nil(t, err)

	err = s.BindAddress("sa1", "ba1")
	require.Nil(t, err)

	// check cache
	skyaddr := s.cache.bindAddress["ba1"]
	require.Equal(t, "sa1", skyaddr)

	btcaddrs := s.cache.skyDepositSeqsIndex["sa1"]
	require.Equal(t, "ba1", btcaddrs[0])

	// check bucekt
	db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bindAddressBkt)
		require.NotNil(t, bkt)
		v := bkt.Get([]byte("ba1"))
		require.Equal(t, "sa1", string(v))

		var addrs []string
		ok, err := getBktValue(tx, skyDepositSeqsIndexBkt, []byte("sa1"), &addrs)
		require.Nil(t, err)
		require.True(t, ok)
		require.Equal(t, "ba1", addrs[0])
		return nil
	})
}

func TestGetBindAddress(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	// init the bind address bucket
	err = s.BindAddress("skyaddr1", "btcaddr1")
	require.Nil(t, err)
	err = s.BindAddress("skyaddr2", "btcaddr2")
	require.Nil(t, err)
	err = s.BindAddress("skyaddr2", "btcaddr3")
	require.Nil(t, err)

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
			addr, ok := s.GetBindAddress(tc.btcAddr)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.expectSkyAddr, addr)
		})
	}
}

func TestGetDepositInfo(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	err = s.AddDepositInfo(DepositInfo{
		BtcTx:      "btx1",
		BtcAddress: "btcaddr1",
		SkyAddress: "skyaddr1",
		Status:     StatusDone,
	})
	require.Nil(t, err)

	dpi, ok := s.GetDepositInfo("btx1")
	require.True(t, ok)
	require.Equal(t, "btcaddr1", dpi.BtcAddress)
	require.Equal(t, "skyaddr1", dpi.SkyAddress)
	require.Equal(t, StatusDone, dpi.Status)
	require.NotEmpty(t, dpi.UpdatedAt)
}

func TestUpdateDeposit(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	err = s.AddDepositInfo(DepositInfo{
		BtcTx:      "btx1",
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr1",
	})
	require.Nil(t, err)

	err = s.AddDepositInfo(DepositInfo{
		BtcTx:      "btx2",
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr2",
	})
	require.Nil(t, err)

	db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(depositInfoBkt)
		v1 := bkt.Get([]byte("btx1"))
		require.NotNil(t, v1)
		var dpi1 DepositInfo
		require.Nil(t, json.Unmarshal(v1, &dpi1))

		require.Equal(t, dpi1.Status, StatusWaitDeposit)

		v2 := bkt.Get([]byte("btx2"))
		require.NotNil(t, v2)
		var dpi2 DepositInfo
		require.Nil(t, json.Unmarshal(v2, &dpi2))
		require.Equal(t, dpi2.Status, StatusWaitDeposit)

		return nil
	})

	err = s.UpdateDepositInfo("btx1", func(dpi DepositInfo) DepositInfo {
		dpi.Status = StatusWaitSend
		dpi.Txid = "121212"

		// try to change immutable value skyaddress
		dpi.SkyAddress = "no change"
		return dpi
	})

	require.Nil(t, err)
	db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(depositInfoBkt)
		v1 := bkt.Get([]byte("btx1"))
		require.NotNil(t, v1)
		var dpi1 DepositInfo
		require.Nil(t, json.Unmarshal(v1, &dpi1))

		// check updated value
		require.Equal(t, dpi1.Status, StatusWaitSend)
		require.Equal(t, "121212", dpi1.Txid)

		// check immutable value
		require.Equal(t, "skyaddr1", dpi1.SkyAddress)
		return nil
	})

	// check cache
	dpi, ok := s.cache.depositInfo["btx1"]
	require.True(t, ok)
	require.Equal(t, StatusWaitSend, dpi.Status)
	require.Equal(t, "skyaddr1", dpi.SkyAddress)
	require.Equal(t, "btcaddr1", dpi.BtcAddress)

	// test no exist deposit info
}

func TestGetDepositInfoOfSkyAddr(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	s.BindAddress("skyaddr1", "btcaddr1")

	dpis, err := s.GetDepositInfoOfSkyAddress("skyaddr1")
	require.Nil(t, err)
	require.Equal(t, 1, len(dpis))
	require.Equal(t, dpis[0].BtcAddress, "btcaddr1")

	s.BindAddress("skyaddr1", "btcaddr2")

	dpis, err = s.GetDepositInfoOfSkyAddress("skyaddr1")
	require.Nil(t, err)
	require.Equal(t, 2, len(dpis))
	require.Equal(t, dpis[0].BtcAddress, "btcaddr1")
	require.Equal(t, dpis[1].BtcAddress, "btcaddr2")
}

func TestLoadCache(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	dis := []DepositInfo{
		DepositInfo{
			BtcTx:      "btx1",
			BtcAddress: "btcaddr1",
			SkyAddress: "skyaddr1",
		},
		DepositInfo{
			BtcTx:      "btx2",
			BtcAddress: "btcaddr2",
			SkyAddress: "skyaddr2",
		},
		DepositInfo{
			BtcTx:      "btx3",
			BtcAddress: "btcaddr2",
			SkyAddress: "skyaddr2",
		},
	}

	s, err := newStore(db)
	for _, dpi := range dis {
		s.BindAddress(dpi.SkyAddress, dpi.BtcAddress)
		s.AddDepositInfo(dpi)
	}

	cache, err := loadCache(db)
	require.Nil(t, err)
	require.NotNil(t, cache.bindAddress)
	require.NotNil(t, cache.depositInfo)
	require.NotNil(t, cache.skyDepositSeqsIndex)
	require.NotNil(t, cache.btcTxs)

	require.Equal(t, "skyaddr1", cache.bindAddress["btcaddr1"])
	require.Equal(t, "skyaddr2", cache.bindAddress["btcaddr2"])

	require.Equal(t, "btcaddr1", cache.skyDepositSeqsIndex["skyaddr1"][0])
	require.Equal(t, "btcaddr2", cache.skyDepositSeqsIndex["skyaddr2"][0])

	require.Equal(t, []string{"btx2", "btx3"}, cache.btcTxs["btcaddr2"])
}

func TestGetDepositInfoArray(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	dpis := []DepositInfo{
		{
			BtcTx:      "t1",
			BtcAddress: "b1",
			SkyAddress: "s1",
			Status:     StatusWaitDeposit,
		},
		{
			BtcTx:      "t2",
			BtcAddress: "b2",
			SkyAddress: "s2",
			Status:     StatusWaitSend,
		},
	}

	for _, dpi := range dpis {
		require.Nil(t, s.AddDepositInfo(dpi))
	}

	ds := s.GetDepositInfoArray(func(dpi DepositInfo) bool {
		return dpi.Status == StatusWaitSend
	})

	require.Equal(t, 1, len(ds))
	require.Equal(t, dpis[1].Status, ds[0].Status)
	require.Equal(t, dpis[1].BtcAddress, ds[0].BtcAddress)
	require.Equal(t, dpis[1].SkyAddress, ds[0].SkyAddress)

	ds1 := s.GetDepositInfoArray(func(dpi DepositInfo) bool {
		return dpi.Status == StatusWaitDeposit
	})

	require.Equal(t, 1, len(ds1))
	require.Equal(t, dpis[0].Status, ds1[0].Status)
	require.Equal(t, dpis[0].BtcAddress, ds1[0].BtcAddress)
	require.Equal(t, dpis[0].SkyAddress, ds1[0].SkyAddress)
}
