package exchange

import (
	"encoding/json"
	"errors"
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

	seq, err := s.AddDepositInfo(DepositInfo{
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr1",
	})
	require.Nil(t, err)
	require.Equal(t, uint64(1), seq)

	// check the deposit info cache
	dpi, ok := s.cache.depositInfo["btcaddr1"]
	require.True(t, ok)
	require.Equal(t, seq, dpi.Seq)
	require.Equal(t, "skyaddr1", dpi.SkyAddress)
	require.Equal(t, "btcaddr1", dpi.BtcAddress)
	require.Equal(t, StatusWaitDeposit, dpi.Status)
	require.NotEmpty(t, dpi.UpdatedAt)

	// check binded address cache
	skyAddr, ok := s.cache.bindAddress["btcaddr1"]
	require.True(t, ok)
	require.Equal(t, "skyaddr1", skyAddr)

	// check sky index cache
	btcAddrs := s.cache.skyDepositSeqsIndex["skyaddr1"]
	require.Equal(t, 1, len(btcAddrs))
	require.Equal(t, "btcaddr1", btcAddrs[0])

	// check in db
	db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(depositInfoBkt).Get([]byte("btcaddr1"))
		require.NotNil(t, v)
		var dpi DepositInfo
		require.Nil(t, json.Unmarshal(v, &dpi))
		require.NotEmpty(t, dpi.UpdatedAt)

		// check skyDepositSeqsIndex
		v = tx.Bucket(skyDepositSeqsIndexBkt).Get([]byte("skyaddr1"))
		require.NotNil(t, v)
		var btcAddrs []string
		require.Nil(t, json.Unmarshal(v, &btcAddrs))
		require.Equal(t, btcAddrs[0], "btcaddr1")

		// check bind address bkt
		v = tx.Bucket(bindAddressBkt).Get([]byte("btcaddr1"))
		require.NotNil(t, v)
		require.Equal(t, "skyaddr1", string(v))
		return nil
	})

	seq, err = s.AddDepositInfo(DepositInfo{
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr2",
	})

	require.Nil(t, err)
	require.Equal(t, uint64(2), seq)
	db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(depositInfoBkt).Get([]byte("btcaddr2"))
		require.NotNil(t, v)
		var dpi DepositInfo
		require.Nil(t, json.Unmarshal(v, &dpi))
		require.NotEmpty(t, dpi.UpdatedAt)

		// check skyDepositSeqsIndex
		v = tx.Bucket(skyDepositSeqsIndexBkt).Get([]byte("skyaddr1"))
		require.NotNil(t, v)
		var btcAddrs []string
		require.Nil(t, json.Unmarshal(v, &btcAddrs))
		require.Equal(t, btcAddrs, []string{"btcaddr1", "btcaddr2"})
		return nil
	})

	// check invalid deposit info
	_, err = s.AddDepositInfo(DepositInfo{})
	require.NotNil(t, errors.New("skycoin address is empty"))
}

func TestGetBindAddress(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	// init the bind address bucket
	_, err = s.AddDepositInfo(DepositInfo{BtcAddress: "btcaddr1", SkyAddress: "skyaddr1"})
	require.Nil(t, err)
	_, err = s.AddDepositInfo(DepositInfo{BtcAddress: "btcaddr2", SkyAddress: "skyaddr2"})
	require.Nil(t, err)
	_, err = s.AddDepositInfo(DepositInfo{BtcAddress: "btcaddr3", SkyAddress: "skyaddr2"})
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

	_, err = s.AddDepositInfo(DepositInfo{
		BtcAddress: "btcaddr1",
		SkyAddress: "skyaddr1",
		Status:     StatusDone,
	})
	require.Nil(t, err)

	dpi, ok := s.GetDepositInfo("btcaddr1")
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

	seq, err := s.AddDepositInfo(DepositInfo{
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr1",
	})
	require.Nil(t, err)
	require.Equal(t, uint64(1), seq)

	seq2, err := s.AddDepositInfo(DepositInfo{
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr2",
	})
	require.Nil(t, err)
	require.Equal(t, uint64(2), seq2)

	db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(depositInfoBkt)
		v1 := bkt.Get([]byte("btcaddr1"))
		require.NotNil(t, v1)
		var dpi1 DepositInfo
		require.Nil(t, json.Unmarshal(v1, &dpi1))

		require.Equal(t, dpi1.Status, StatusWaitDeposit)

		v2 := bkt.Get([]byte("btcaddr2"))
		require.NotNil(t, v2)
		var dpi2 DepositInfo
		require.Nil(t, json.Unmarshal(v2, &dpi2))
		require.Equal(t, dpi2.Status, StatusWaitDeposit)

		return nil
	})

	err = s.UpdateDepositInfo("btcaddr1", func(dpi DepositInfo) DepositInfo {
		dpi.Status = StatusWaitSend
		dpi.Txid = "121212"

		// try to change immutable value skyaddress
		dpi.SkyAddress = "no change"
		return dpi
	})

	require.Nil(t, err)
	db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(depositInfoBkt)
		v1 := bkt.Get([]byte("btcaddr1"))
		require.NotNil(t, v1)
		var dpi1 DepositInfo
		require.Nil(t, json.Unmarshal(v1, &dpi1))

		// check updated value
		require.Equal(t, dpi1.Status, StatusWaitSend)
		require.Equal(t, "121212", dpi1.Txid)

		// check immutable value
		require.Equal(t, "skyaddr1", dpi1.SkyAddress)
		require.Equal(t, seq, dpi1.Seq)
		return nil
	})

	// check cache
	dpi, ok := s.cache.depositInfo["btcaddr1"]
	require.True(t, ok)
	require.Equal(t, seq, dpi.Seq)
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

	s.AddDepositInfo(DepositInfo{
		BtcAddress: "btcaddr1",
		SkyAddress: "skyaddr1",
	})

	dpis, err := s.GetDepositInfoOfSkyAddress("skyaddr1")
	require.Nil(t, err)
	require.Equal(t, 1, len(dpis))
	require.Equal(t, dpis[0].BtcAddress, "btcaddr1")

	s.AddDepositInfo(DepositInfo{
		BtcAddress: "btcaddr2",
		SkyAddress: "skyaddr1",
	})

	dpis, err = s.GetDepositInfoOfSkyAddress("skyaddr1")
	require.Nil(t, err)
	require.Equal(t, 2, len(dpis))
	require.Equal(t, dpis[0].BtcAddress, "btcaddr1")
	require.Equal(t, dpis[1].BtcAddress, "btcaddr2")
}

func TestLoadCache(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	s.AddDepositInfo(DepositInfo{
		BtcAddress: "btcaddr1",
		SkyAddress: "skyaddr1",
	})

	s.AddDepositInfo(DepositInfo{
		BtcAddress: "btcaddr2",
		SkyAddress: "skyaddr2",
	})

	cache, err := loadCache(db)
	require.Nil(t, err)
	require.NotNil(t, cache.bindAddress)
	require.NotNil(t, cache.depositInfo)
	require.NotNil(t, cache.skyDepositSeqsIndex)
}

func TestGetDepositInfoArray(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	dpis := []DepositInfo{
		{
			BtcAddress: "b1",
			SkyAddress: "s1",
			Status:     StatusWaitDeposit,
		},
		{
			BtcAddress: "b2",
			SkyAddress: "s2",
			Status:     StatusWaitSend,
		},
	}

	for _, dpi := range dpis {
		s.AddDepositInfo(dpi)
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
