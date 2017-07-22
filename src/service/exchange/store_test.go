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
	f := fmt.Sprintf("test%d.db", rand.Intn(1024))
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

// func TestBindAddress(t *testing.T) {
// 	db, shutdown := setupDB(t)
// 	defer shutdown()

// 	s, err := newStore(db)
// 	require.Nil(t, err)

// 	// bind success
// 	require.Nil(t, s.BindAddress("btcaddr1", "skyaddr1"))
// 	require.Equal(t, s.cache.bindAddress["btcaddr1"], "skyaddr1")

// 	// bind dup addr
// 	err = s.BindAddress("btcaddr1", "skyaddr2")
// 	require.Equal(t, errors.New("address btcaddr1 already binded"), err)
// }

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

	seq, err := s.AddDepositInfo(depositInfo{
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr1",
	})
	require.Nil(t, err)
	require.Equal(t, uint64(1), seq)

	// check the deposit info cache
	dpi, ok := s.cache.depositInfo[seq]
	require.True(t, ok)
	require.Equal(t, seq, dpi.Seq)
	require.Equal(t, "skyaddr1", dpi.SkyAddress)
	require.Equal(t, "btcaddr1", dpi.BtcAddress)
	require.Equal(t, statusWaitBtcDeposit, dpi.Status)
	require.NotEmpty(t, dpi.UpdatedAt)

	// check binded address cache
	skyAddr, ok := s.cache.bindAddress["btcaddr1"]
	require.True(t, ok)
	require.Equal(t, "skyaddr1", skyAddr)

	// check sky index cache
	seqs := s.cache.skyDepositSeqsIndex["skyaddr1"]
	require.Equal(t, 1, len(seqs))
	require.Equal(t, seq, seqs[0])

	// check in db
	db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(depositInfoBkt).Get(uint64ToBytes(seq))
		require.NotNil(t, v)
		var dpi depositInfo
		require.Nil(t, json.Unmarshal(v, &dpi))
		require.NotEmpty(t, dpi.UpdatedAt)

		// check skyDepositSeqsIndex
		v = tx.Bucket(skyDepositSeqsIndexBkt).Get([]byte("skyaddr1"))
		require.NotNil(t, v)
		var seqs []uint64
		require.Nil(t, json.Unmarshal(v, &seqs))
		require.Equal(t, seqs[0], seq)

		// check bind address bkt
		v = tx.Bucket(bindAddressBkt).Get([]byte("btcaddr1"))
		require.NotNil(t, v)
		require.Equal(t, "skyaddr1", string(v))
		return nil
	})

	seq, err = s.AddDepositInfo(depositInfo{
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr2",
	})
	require.Nil(t, err)
	require.Equal(t, uint64(2), seq)
	db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(depositInfoBkt).Get(uint64ToBytes(seq))
		require.NotNil(t, v)
		var dpi depositInfo
		require.Nil(t, json.Unmarshal(v, &dpi))
		require.NotEmpty(t, dpi.UpdatedAt)

		// check skyDepositSeqsIndex
		v = tx.Bucket(skyDepositSeqsIndexBkt).Get([]byte("skyaddr1"))
		require.NotNil(t, v)
		var seqs []uint64
		require.Nil(t, json.Unmarshal(v, &seqs))
		require.Equal(t, seqs, []uint64{1, 2})
		return nil
	})

	// check invalid deposit info
	_, err = s.AddDepositInfo(depositInfo{})
	require.NotNil(t, errors.New("skycoin address is empty"))
}

func TestGetBindAddress(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	// init the bind address bucket
	_, err = s.AddDepositInfo(depositInfo{BtcAddress: "btcaddr1", SkyAddress: "skyaddr1"})
	require.Nil(t, err)
	_, err = s.AddDepositInfo(depositInfo{BtcAddress: "btcaddr2", SkyAddress: "skyaddr2"})
	require.Nil(t, err)
	_, err = s.AddDepositInfo(depositInfo{BtcAddress: "btcaddr3", SkyAddress: "skyaddr2"})
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
func TestUpdateDepositStatus(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	seq, err := s.AddDepositInfo(depositInfo{
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr1",
	})
	require.Nil(t, err)
	require.Equal(t, uint64(1), seq)

	seq2, err := s.AddDepositInfo(depositInfo{
		SkyAddress: "skyaddr1",
		BtcAddress: "btcaddr2",
	})
	require.Nil(t, err)
	require.Equal(t, uint64(2), seq2)

	db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(depositInfoBkt)
		v1 := bkt.Get(uint64ToBytes(seq))
		require.NotNil(t, v1)
		var dpi1 depositInfo
		require.Nil(t, json.Unmarshal(v1, &dpi1))

		require.Equal(t, dpi1.Status, statusWaitBtcDeposit)

		v2 := bkt.Get(uint64ToBytes(seq2))
		require.NotNil(t, v2)
		var dpi2 depositInfo
		require.Nil(t, json.Unmarshal(v2, &dpi2))
		require.Equal(t, dpi2.Status, statusWaitBtcDeposit)

		return nil
	})

	err = s.UpdateDepositStatus(seq, statusWaitSkySend)
	require.Nil(t, err)

	db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(depositInfoBkt)
		v1 := bkt.Get(uint64ToBytes(seq))
		require.NotNil(t, v1)
		var dpi1 depositInfo
		require.Nil(t, json.Unmarshal(v1, &dpi1))

		require.Equal(t, dpi1.Status, statusWaitSkySend)
		return nil
	})

	// check cache
	dpi, ok := s.cache.depositInfo[seq]
	require.True(t, ok)
	require.Equal(t, seq, dpi.Seq)
	require.Equal(t, statusWaitSkySend, dpi.Status)
}

func TestGetDepositInfoOfSkyAddr(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	s.AddDepositInfo(depositInfo{
		BtcAddress: "btcaddr1",
		SkyAddress: "skyaddr1",
	})

	dpis, err := s.GetDepositInfoOfSkyAddress("skyaddr1")
	require.Nil(t, err)
	require.Equal(t, 1, len(dpis))
	require.Equal(t, dpis[0].BtcAddress, "btcaddr1")

	s.AddDepositInfo(depositInfo{
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

	s.AddDepositInfo(depositInfo{
		BtcAddress: "btcaddr1",
		SkyAddress: "skyaddr1",
	})

	s.AddDepositInfo(depositInfo{
		BtcAddress: "btcaddr2",
		SkyAddress: "skyaddr2",
	})

	cache, err := loadCache(db)
	require.Nil(t, err)
	require.NotNil(t, cache.bindAddress)
	require.NotNil(t, cache.depositInfo)
	require.NotNil(t, cache.skyDepositSeqsIndex)
}
