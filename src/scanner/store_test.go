package scanner

import (
	"errors"
	"sort"
	"testing"

	"github.com/boltdb/bolt"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/dbutil"
	"github.com/skycoin/teller/src/util/testutil"
)

type MockStore struct {
	mock.Mock
}

func (m *MockStore) GetScanAddresses(coinType string) ([]string, error) {
	args := m.Called(coinType)

	addrs := args.Get(0)

	if addrs == nil {
		return nil, args.Error(1)
	}

	return addrs.([]string), args.Error(1)
}

func (m *MockStore) AddScanAddress(addr, coinType string) error {
	args := m.Called(addr, coinType)
	return args.Error(1)
}

func (m *MockStore) SetDepositProcessed(dv string) error {
	args := m.Called(dv)
	return args.Error(1)
}

func (m *MockStore) GetUnprocessedDeposits() ([]Deposit, error) {
	args := m.Called()

	dvs := args.Get(0)

	if dvs == nil {
		return nil, args.Error(1)
	}

	return dvs.([]Deposit), args.Error(1)
}

func (m *MockStore) ScanBlock(*btcjson.GetBlockVerboseResult) ([]Deposit, error) {
	args := m.Called()

	dvs := args.Get(0)

	if dvs == nil {
		return nil, args.Error(1)
	}

	return dvs.([]Deposit), args.Error(1)
}

func TestBtcTxN(t *testing.T) {
	d := Deposit{
		Tx: "foo",
		N:  2,
	}

	require.Equal(t, "foo:2", d.ID())
}

func TestNewStore(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	s, err := NewStore(log, db)
	s.AddSupportedCoin(CoinTypeBTC)
	require.NoError(t, err)

	s.db.View(func(tx *bolt.Tx) error {
		scanBktFullName := dbutil.ByteJoin(scanMetaBktPrefix, CoinTypeBTC, "_")
		bkt := tx.Bucket(scanBktFullName)
		require.NotNil(t, bkt)

		require.NotNil(t, tx.Bucket(depositBkt))

		return nil
	})
}

func TestGetDepositAddresses(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	s, err := NewStore(log, db)
	s.AddSupportedCoin(CoinTypeBTC)
	require.NoError(t, err)

	var addrs = []string{
		"s1",
		"s2",
		"s3",
	}

	for _, a := range addrs {
		err := s.AddScanAddress(a, CoinTypeBTC)
		require.NoError(t, err)
	}

	as, err := s.GetScanAddresses(CoinTypeBTC)
	require.NoError(t, err)

	sort.Strings(as)
	sort.Strings(addrs)

	require.Equal(t, addrs, as)

	// check db
	s.db.View(func(tx *bolt.Tx) error {
		var as []string
		scanBktFullName := dbutil.ByteJoin(scanMetaBktPrefix, CoinTypeBTC, "_")
		err := dbutil.GetBucketObject(tx, scanBktFullName, depositAddressesKey, &as)
		require.NoError(t, err)

		require.Equal(t, len(addrs), len(as))

		for _, a := range addrs {
			require.Contains(t, as, a)
		}

		return nil
	})
}

func TestAddDepositAddress(t *testing.T) {
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
			s.AddSupportedCoin(CoinTypeBTC)
			require.NoError(t, err)

			err = db.Update(func(tx *bolt.Tx) error {
				scanBktFullName := dbutil.ByteJoin(scanMetaBktPrefix, CoinTypeBTC, "_")
				return dbutil.PutBucketValue(tx, scanBktFullName, depositAddressesKey, tc.initAddrs)
			})
			require.NoError(t, err)

			for _, a := range tc.addAddrs {
				if er := s.AddScanAddress(a, CoinTypeBTC); er != nil {
					err = er
				}
			}

			require.Equal(t, tc.err, err)
		})
	}
}

func TestPushDeposit(t *testing.T) {
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
	s.AddSupportedCoin(CoinTypeBTC)
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

func TestPutBktValue(t *testing.T) {

	type kv struct {
		key   string
		value Deposit
	}

	dvs := []Deposit{
		Deposit{
			Tx: "t1",
			N:  1,
		},
		Deposit{
			Tx: "t2",
			N:  2,
		},
		Deposit{
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

func TestGetBktValue(t *testing.T) {

	type kv struct {
		key   string
		value Deposit
	}

	dvs := []Deposit{
		Deposit{
			Tx: "t1",
			N:  1,
		},
		Deposit{
			Tx: "t2",
			N:  2,
		},
		Deposit{
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
					dbutil.PutBucketValue(tx, bktName, kv.key, kv.value)
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

func TestScanBlock(t *testing.T) {
	// TODO
}
