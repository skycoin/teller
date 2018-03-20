package scanner

import (
	"errors"
	"sort"
	"testing"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/require"

	"github.com/MDLlife/teller/src/util/dbutil"
	"github.com/MDLlife/teller/src/util/testutil"
)

func TestWAVESTxN(t *testing.T) {
	d := Deposit{
		Tx: "foo",
		N:  2,
	}

	require.Equal(t, "foo:2", d.ID())
}

func TestWAVESNewStore(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	s, err := NewStore(log, db)
	require.NoError(t, err)
	err = s.AddSupportedCoin(CoinTypeWAVES)
	require.NoError(t, err)

	err = s.db.View(func(tx *bolt.Tx) error {
		scanBktFullName := MustGetScanMetaBkt(CoinTypeWAVES)
		bkt := tx.Bucket(scanBktFullName)
		require.NotNil(t, bkt)

		require.NotNil(t, tx.Bucket(DepositBkt))

		return nil
	})
	require.NoError(t, err)
}

func TestWAVESGetDepositAddresses(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	s, err := NewStore(log, db)
	require.NoError(t, err)
	err = s.AddSupportedCoin(CoinTypeWAVES)
	require.NoError(t, err)

	var addrs = []string{
		"s1",
		"s2",
		"s3",
	}

	for _, a := range addrs {
		err := s.AddScanAddress(a, CoinTypeWAVES)
		require.NoError(t, err)
	}

	as, err := s.GetScanAddresses(CoinTypeWAVES)
	require.NoError(t, err)

	sort.Strings(as)
	sort.Strings(addrs)

	require.Equal(t, addrs, as)

	// check db
	err = s.db.View(func(tx *bolt.Tx) error {
		var as []string
		scanBktFullName := MustGetScanMetaBkt(CoinTypeWAVES)
		err := dbutil.GetBucketObject(tx, scanBktFullName, depositAddressesKey, &as)
		require.NoError(t, err)

		require.Equal(t, len(addrs), len(as))

		for _, a := range addrs {
			require.Contains(t, as, a)
		}

		return nil
	})
	require.NoError(t, err)
}

func TestWAVESAddDepositAddress(t *testing.T) {
	addrs := []string{
		"a1",
		"a2",
		"a3",
		"a4",
	}

	var testCases = []struct {
		name        string
		initAddrs   []string
		addAddrs    []string
		expectAddrs []string
		err         error
	}{
		{
			"ok",
			addrs[:1],
			addrs[1:2],
			addrs[:2],
			nil,
		},
		{
			"dup",
			addrs[:2],
			addrs[1:2],
			[]string{},
			NewDuplicateDepositAddressErr(addrs[1]),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, shutdown := testutil.PrepareDB(t)
			defer shutdown()
			log, _ := testutil.NewLogger(t)

			s, err := NewStore(log, db)
			require.NoError(t, err)
			err = s.AddSupportedCoin(CoinTypeWAVES)
			require.NoError(t, err)

			err = db.Update(func(tx *bolt.Tx) error {
				scanBktFullName := MustGetScanMetaBkt(CoinTypeWAVES)
				return dbutil.PutBucketValue(tx, scanBktFullName, depositAddressesKey, tc.initAddrs)
			})
			require.NoError(t, err)

			for _, a := range tc.addAddrs {
				if er := s.AddScanAddress(a, CoinTypeWAVES); er != nil {
					err = er
				}
			}

			require.Equal(t, tc.err, err)
		})
	}
}

func TestWAVESPushDeposit(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	dvs := []Deposit{
		{
			Address: "b1",
			Value:   1,
			Height:  1,
			Tx:      "t1",
			N:       1,
		},
		{
			Address: "b2",
			Value:   2,
			Height:  2,
			Tx:      "t2",
			N:       2,
		},
	}

	keyMap := make(map[string]struct{})
	for _, dv := range dvs {
		keyMap[dv.ID()] = struct{}{}
	}

	log, _ := testutil.NewLogger(t)

	s, err := NewStore(log, db)
	require.NoError(t, err)
	err = s.AddSupportedCoin(CoinTypeWAVES)
	require.NoError(t, err)

	err = db.Update(func(tx *bolt.Tx) error {
		for _, dv := range dvs {
			err := s.pushDepositTx(tx, dv)
			require.NoError(t, err)
		}
		return nil
	})

	require.NoError(t, err)
}

func TestWAVESPutBktValue(t *testing.T) {

	type kv struct {
		key   string
		value Deposit
	}

	dvs := []Deposit{
		{
			Tx: "t1",
			N:  1,
		},
		{
			Tx: "t2",
			N:  2,
		},
		{
			Tx: "t3",
			N:  3,
		},
	}

	init := []kv{
		{
			"k1",
			dvs[0],
		},
		{
			"k2",
			dvs[1],
		},
		{
			"k3",
			dvs[2],
		},
	}

	bktName := []byte("test")

	tt := []struct {
		name    string
		putV    []kv
		key     string
		expectV Deposit
		err     error
	}{
		{
			"normal",
			init,
			"k1",
			dvs[0],
			nil,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			db, shutdown := testutil.PrepareDB(t)
			defer shutdown()

			err := db.Update(func(tx *bolt.Tx) error {
				_, err := tx.CreateBucketIfNotExists(bktName)
				require.NoError(t, err)

				for _, kv := range tc.putV {
					err := dbutil.PutBucketValue(tx, bktName, kv.key, kv.value)
					require.Equal(t, tc.err, err)
				}

				return nil
			})

			require.NoError(t, err)

			err = db.View(func(tx *bolt.Tx) error {
				var dv Deposit
				require.Nil(t, dbutil.GetBucketObject(tx, bktName, tc.key, &dv))
				require.Equal(t, tc.expectV, dv)
				return nil
			})

			require.NoError(t, err)
		})
	}
}

func TestWAVESGetBktValue(t *testing.T) {

	type kv struct {
		key   string
		value Deposit
	}

	dvs := []Deposit{
		{
			Tx: "t1",
			N:  1,
		},
		{
			Tx: "t2",
			N:  2,
		},
		{
			Tx: "t3",
			N:  3,
		},
	}

	init := []kv{
		{
			"k1",
			dvs[0],
		},
		{
			"k2",
			dvs[1],
		},
		{
			"k3",
			dvs[2],
		},
	}

	bktName := []byte("test")

	tt := []struct {
		name    string
		init    []kv
		key     string
		v       interface{}
		expectV Deposit
		err     error
	}{
		{
			"normal",
			init,
			"k1",
			&Deposit{},
			dvs[0],
			nil,
		},
		{
			"not exist",
			init,
			"k5",
			&Deposit{},
			dvs[0],
			dbutil.NewObjectNotExistErr(bktName, []byte("k5")),
		},
		{
			"invalid accept value",
			init,
			"k3",
			Deposit{},
			dvs[0],
			errors.New("decode value failed: json: Unmarshal(non-pointer scanner.Deposit)"),
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			db, shutdown := testutil.PrepareDB(t)
			defer shutdown()

			err := db.Update(func(tx *bolt.Tx) error {
				_, err := tx.CreateBucketIfNotExists(bktName)
				require.NoError(t, err)

				for _, kv := range tc.init {
					err := dbutil.PutBucketValue(tx, bktName, kv.key, kv.value)
					require.NoError(t, err)
				}

				return nil
			})

			require.NoError(t, err)

			err = db.View(func(tx *bolt.Tx) error {
				err := dbutil.GetBucketObject(tx, bktName, tc.key, tc.v)
				require.Equal(t, tc.err, err)

				if err == nil {
					v := tc.v.(*Deposit)
					require.Equal(t, tc.expectV, *v)
				}

				return nil
			})

			require.NoError(t, err)
		})
	}
}

func TestWAVESScanBlock(t *testing.T) {
	// TODO
}
