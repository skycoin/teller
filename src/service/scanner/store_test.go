package scanner

import (
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/dbutil"
	"github.com/skycoin/teller/src/service/testutil"
)

func TestNewStore(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.NoError(t, err)

	s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(scanMetaBkt)
		require.NotNil(t, bkt)

		require.NotNil(t, tx.Bucket(depositValueBkt))

		return nil
	})
}

func TestGetLastScanBlock(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.NoError(t, err)

	lsb, err := s.getLastScanBlock()
	require.NoError(t, err)
	require.Equal(t, "", lsb.Hash)
	require.Equal(t, int64(0), lsb.Height)

	scanBlock := LastScanBlock{
		Hash:   "00000000000004509071260531df744090422d372d706cee907b2b5f2be8b8ff",
		Height: 222597,
	}

	err = s.setLastScanBlock(scanBlock)
	require.NoError(t, err)

	lsb, err = s.getLastScanBlock()
	require.NoError(t, err)
	require.Equal(t, scanBlock, lsb)
}

func TestSetLastScanBlock(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.NoError(t, err)

	scanBlocks := []LastScanBlock{
		LastScanBlock{
			Hash:   "00000000000004509071260531df744090422d372d706cee907b2b5f2be8b8ff",
			Height: 222597,
		},
		LastScanBlock{
			Hash:   "000000000000003f499b9736635dd65101c4c70aef4912b5c5b4b86cd36b4d27",
			Height: 222618,
		},
	}

	require.Nil(t, s.setLastScanBlock(scanBlocks[0]))
	lsb, err := s.getLastScanBlock()
	require.NoError(t, err)
	require.Equal(t, scanBlocks[0], lsb)

	require.Nil(t, s.setLastScanBlock(scanBlocks[1]))
	lsb, err = s.getLastScanBlock()
	require.NoError(t, err)
	require.Equal(t, scanBlocks[1], lsb)
}

func TestGetDepositAddresses(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.NoError(t, err)

	var addrs = []string{
		"s1",
		"s2",
		"s3",
	}

	for _, a := range addrs {
		err := s.addScanAddress(a)
		require.NoError(t, err)
	}

	as, err := s.getScanAddresses()
	require.NoError(t, err)

	sort.Strings(as)
	sort.Strings(addrs)

	require.Equal(t, addrs, as)

	// check db
	s.db.View(func(tx *bolt.Tx) error {
		var as []string
		err := dbutil.GetBucketObject(tx, scanMetaBkt, depositAddressesKey, &as)
		require.NoError(t, err)

		require.Equal(t, len(addrs), len(as))

		for _, a := range addrs {
			require.Contains(t, as, a)
		}

		return nil
	})
}

func TestAddDepositeAddress(t *testing.T) {
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
			s, err := newStore(db)
			require.NoError(t, err)

			err = db.Update(func(tx *bolt.Tx) error {
				return dbutil.PutBucketValue(tx, scanMetaBkt, depositAddressesKey, tc.initAddrs)
			})
			require.NoError(t, err)

			for _, a := range tc.addAddrs {
				if er := s.addScanAddress(a); er != nil {
					err = er
				}
			}

			require.Equal(t, tc.err, err)
		})
	}
}

func TestPushDepositValue(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	dvs := []DepositValue{
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
		keyMap[fmt.Sprintf("%v:%v", dv.Tx, dv.N)] = struct{}{}
	}

	s, err := newStore(db)
	require.NoError(t, err)

	err = db.Update(func(tx *bolt.Tx) error {
		for _, dv := range dvs {
			err := s.pushDepositValueTx(tx, dv)
			require.NoError(t, err)
		}
		return nil
	})

	require.NoError(t, err)

	// check db
	idxs, err := s.getDepositValueIndex()
	require.NoError(t, err)
	require.Len(t, idxs, len(keyMap))
	for _, idx := range idxs {
		require.Contains(t, keyMap, idx)
	}
}

func TestPopDepositValue(t *testing.T) {
	dvs := []DepositValue{
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

	tt := []struct {
		name  string
		init  []DepositValue
		popV  DepositValue
		popOk bool
	}{
		{
			"normal pop",
			dvs[:],
			dvs[0],
			true,
		},
		{
			"pop empty",
			dvs[:0],
			DepositValue{},
			false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			db, shutdown := testutil.PrepareDB(t)
			defer shutdown()

			s, err := newStore(db)
			require.NoError(t, err)

			err = db.Update(func(tx *bolt.Tx) error {
				for _, dv := range tc.init {
					err := s.pushDepositValueTx(tx, dv)
					require.NoError(t, err)
					if err != nil {
						return err
					}
				}
				return nil
			})

			require.NoError(t, err)

			dv, err := s.popDepositValue()

			if !tc.popOk {
				require.Error(t, err)
				require.IsType(t, DepositValuesEmptyErr{}, err)
				return
			}

			require.NoError(t, err)

			tc.popV.IsUsed = true
			require.Equal(t, tc.popV, dv)

			// check db
			err = db.View(func(tx *bolt.Tx) error {
				idxs, err := s.getDepositValueIndexTx(tx)
				require.NoError(t, err)
				if err != nil {
					return err
				}

				require.Len(t, idxs, len(tc.init)-1)

				// key should already been removed from index, have a check
				key := dv.TxN()
				for _, idx := range idxs {
					require.NotEqual(t, idx, key)
				}

				var dv DepositValue
				err = dbutil.GetBucketObject(tx, depositValueBkt, key, &dv)
				require.Nil(t, err)
				require.True(t, dv.IsUsed)

				return nil
			})

			require.NoError(t, err)
		})
	}
}

func TestPutBktValue(t *testing.T) {

	type kv struct {
		key   string
		value DepositValue
	}

	dvs := []DepositValue{
		DepositValue{
			Tx: "t1",
			N:  1,
		},
		DepositValue{
			Tx: "t2",
			N:  2,
		},
		DepositValue{
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
		expectV DepositValue
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
				var dv DepositValue
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
		value DepositValue
	}

	dvs := []DepositValue{
		DepositValue{
			Tx: "t1",
			N:  1,
		},
		DepositValue{
			Tx: "t2",
			N:  2,
		},
		DepositValue{
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
		expectV DepositValue
		err     error
	}{
		{
			"normal",
			init,
			"k1",
			&DepositValue{},
			dvs[0],
			nil,
		},
		{
			"not exist",
			init,
			"k5",
			&DepositValue{},
			dvs[0],
			dbutil.NewObjectNotExistErr(bktName, []byte("k5")),
		},
		{
			"invalid accept value",
			init,
			"k3",
			DepositValue{},
			dvs[0],
			errors.New("decode value failed: json: Unmarshal(non-pointer scanner.DepositValue)"),
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
					v := tc.v.(*DepositValue)
					require.Equal(t, tc.expectV, *v)
				}

				return nil
			})

			require.NoError(t, err)
		})
	}
}
