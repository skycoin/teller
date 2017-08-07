package scanner

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"encoding/json"

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

	s, err := newStore(db)
	require.Nil(t, err)

	s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(scanMetaBkt)
		require.NotNil(t, bkt)

		require.NotNil(t, tx.Bucket(depositValueBkt))

		return nil
	})

	require.NotNil(t, s.cache)
}

func TestGetLastScanBlock(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	hash, height, err := s.getLastScanBlock()
	require.Nil(t, err)
	require.Equal(t, "", hash)
	require.Equal(t, int64(0), height)

	scanBlock := lastScanBlock{
		Hash:   "00000000000004509071260531df744090422d372d706cee907b2b5f2be8b8ff",
		Height: 222597,
	}

	s.setLastScanBlock(scanBlock)

	require.Nil(t, err)

	h1, height, err := s.getLastScanBlock()
	require.Nil(t, err)
	require.Equal(t, scanBlock.Hash, h1)
	require.Equal(t, scanBlock.Height, height)
}

func TestSetLastScanBlock(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	scanBlocks := []lastScanBlock{
		lastScanBlock{
			Hash:   "00000000000004509071260531df744090422d372d706cee907b2b5f2be8b8ff",
			Height: 222597,
		},
		lastScanBlock{
			Hash:   "000000000000003f499b9736635dd65101c4c70aef4912b5c5b4b86cd36b4d27",
			Height: 222618,
		},
	}

	require.Nil(t, s.setLastScanBlock(scanBlocks[0]))
	hash, height, err := s.getLastScanBlock()
	require.Nil(t, err)
	require.Equal(t, scanBlocks[0].Hash, hash)
	require.Equal(t, scanBlocks[0].Height, height)

	require.Nil(t, s.setLastScanBlock(scanBlocks[1]))
	hash, height, err = s.getLastScanBlock()
	require.Nil(t, err)
	require.Equal(t, scanBlocks[1].Hash, hash)
	require.Equal(t, scanBlocks[1].Height, height)
}

func TestGetDepositAddresses(t *testing.T) {
	db, shutdown := setupDB(t)
	defer shutdown()

	s, err := newStore(db)
	require.Nil(t, err)

	var addrs = []string{
		"s1",
		"s2",
		"s3",
	}

	for _, a := range addrs {
		require.Nil(t, s.addScanAddress(a))
	}

	as := s.getScanAddresses()
	for _, a := range addrs {
		var ok bool
		for _, a1 := range as {
			if a == a1 {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("%s doesn't exist", a)
		}
	}

	// check db
	s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(scanMetaBkt)
		require.NotNil(t, bkt)
		v := bkt.Get(depositAddressesKey)
		require.NotNil(t, v)

		var ads1 []string
		err := json.Unmarshal(v, &ads1)
		require.Nil(t, err)
		require.Equal(t, len(addrs), len(ads1))

		for _, a := range addrs {
			var ok bool
			for _, a1 := range ads1 {
				if a == a1 {
					ok = true
				}
			}
			if !ok {
				t.Fatalf("%s doesn't exist", a)
			}
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
			dupDepositAddrErr(addrs[1]),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, shutdown := setupDB(t)
			defer shutdown()
			s, err := newStore(db)
			require.Nil(t, err)

			err = db.Update(func(tx *bolt.Tx) error {
				v, err := json.Marshal(tc.initAddrs)
				require.Nil(t, err)
				return tx.Bucket(scanMetaBkt).Put(depositAddressesKey, v)
			})
			require.Nil(t, err)

			for _, a := range tc.addAddrs {
				if er := s.addScanAddress(a); er != nil {
					err = er
				}
			}

			require.Equal(t, tc.err, err)
		})
	}
}

func TestNewCache(t *testing.T) {
	c := newCache()
	require.NotNil(t, c.scanAddresses)
}

func TestCacheAddScanAddress(t *testing.T) {
	c := newCache()

	_, ok := c.scanAddresses["a1"]
	require.False(t, ok)

	c.addScanAddress("a1")

	_, ok = c.scanAddresses["a1"]
	require.True(t, ok)
}

func TestCacheGetScanAddresses(t *testing.T) {
	c := newCache()
	ads := []string{
		"a1",
		"a2",
		"a3",
	}

	for _, a := range ads {
		c.addScanAddress(a)
	}

	addrs := c.getScanAddreses()
	require.Equal(t, 3, len(addrs))

	for _, a := range ads {
		var ok bool
		for _, a1 := range addrs {
			if a == a1 {
				ok = true
				break
			}
		}
		if !ok {
			t.Fatalf("%s does not returned", a)
		}
	}
}

func TestCacheLastScanBlock(t *testing.T) {
	c := newCache()
	lsb := lastScanBlock{
		Hash:   "h1",
		Height: 1,
	}
	c.setLastScanBlock(lsb)

	require.Equal(t, lsb, c.lastScanBlock)

	require.Equal(t, lsb, c.getLastScanBlock())
}

func TestCachePushDepositValue(t *testing.T) {
	c := newCache()
	dvs := []DepositValue{
		{
			Address: "b1",
			Value:   1,
			Height:  1,
		},
		{
			Address: "b2",
			Value:   2,
			Height:  2,
		},
		{
			Address: "b3",
			Value:   3,
			Height:  3,
		},
	}

	for _, dv := range dvs {
		c.pushDepositValue(dv)
	}

	require.Equal(t, dvs, c.depositValues)
}

func TestCachePopDepositValue(t *testing.T) {
	c := newCache()
	dvs := []DepositValue{
		{
			Address: "b1",
			Value:   1,
			Height:  1,
		},
		{
			Address: "b2",
			Value:   2,
			Height:  2,
		},
	}

	for _, dv := range dvs {
		c.pushDepositValue(dv)
	}

	require.Equal(t, dvs, c.depositValues)

	dv, ok := c.popDepositValue()
	require.True(t, ok)
	require.Equal(t, dvs[0], dv)
	require.Equal(t, dvs[1:], c.depositValues)

	dv, ok = c.popDepositValue()
	require.True(t, ok)
	require.Equal(t, dvs[1], dv)
	require.Equal(t, 0, len(c.depositValues))

	_, ok = c.popDepositValue()
	require.False(t, ok)
}

func TestPushDepositValue(t *testing.T) {
	db, shutdown := setupDB(t)
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
	require.Nil(t, err)

	for _, dv := range dvs {
		require.Nil(t, s.pushDepositValue(dv))
	}

	// check db
	db.View(func(tx *bolt.Tx) error {
		metaBkt := tx.Bucket(scanMetaBkt)
		require.NotNil(t, metaBkt)

		v := metaBkt.Get(dvIndexListKey)
		require.NotNil(t, v)

		var idxs []string
		require.Nil(t, json.Unmarshal(v, &idxs))

		for _, idx := range idxs {
			_, ok := keyMap[idx]
			require.True(t, ok)
		}

		return nil
	})

	// check cache
	require.Equal(t, dvs, s.cache.depositValues)

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
			db, shutdown := setupDB(t)
			defer shutdown()

			s, err := newStore(db)
			require.Nil(t, err)

			for _, dv := range tc.init {
				require.Nil(t, s.pushDepositValue(dv))
			}

			dv, ok, err := s.popDepositValue()
			require.Nil(t, err)
			require.Equal(t, tc.popOk, ok)
			if ok {
				require.Equal(t, tc.popV, dv)

				// check db
				db.View(func(tx *bolt.Tx) error {
					metaBkt := tx.Bucket(scanMetaBkt)
					require.NotNil(t, metaBkt)

					v := metaBkt.Get(dvIndexListKey)
					require.NotNil(t, v)
					var idxs []string
					require.Nil(t, json.Unmarshal(v, &idxs))
					require.Equal(t, len(tc.init)-1, len(idxs))

					// key should already been removed from index, have a check
					key := fmt.Sprintf("%v:%v", dv.Tx, dv.N)
					var exist bool
					for _, idx := range idxs {
						if idx == key {
							exist = true
							break
						}
					}

					require.False(t, exist)

					// require.Equal(t, idxs[0], fmt.Sprintf("%v:%v", dvs[1].Tx, dvs[1].N))
					var dv DepositValue
					require.Nil(t, getBktValue(tx, depositValueBkt, []byte(key), &dv))
					require.Equal(t, true, dv.IsUsed)

					return nil
				})

				// check cache
				for _, cdv := range s.cache.depositValues {
					if cdv == dv {
						t.Fatalf("%v should be removed from cache", dv)
						return
					}
				}
				return
			}

			// ok is false, means no more value to pop
			require.Equal(t, 0, len(s.cache.depositValues))

			// check db
			db.View(func(tx *bolt.Tx) error {
				// check index bucket
				metaBkt := tx.Bucket(scanMetaBkt)
				require.NotNil(t, metaBkt)
				v := metaBkt.Get(dvIndexListKey)
				if v != nil {
					var idxs []string
					require.Nil(t, json.Unmarshal(v, &idxs))
					require.Equal(t, 0, len(idxs))
				}

				return nil
			})
		})
	}
}

func TestPutBktValue(t *testing.T) {

	type kv struct {
		key   []byte
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
			[]byte("k1"),
			dvs[0],
		},
		{
			[]byte("k2"),
			dvs[1],
		},
		{
			[]byte("k3"),
			dvs[2],
		},
	}

	bktName := []byte("test")

	tt := []struct {
		name    string
		putV    []kv
		key     []byte
		expectV DepositValue
		err     error
	}{
		{
			"normal",
			init,
			[]byte("k1"),
			dvs[0],
			nil,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			db, shutdown := setupDB(t)
			defer shutdown()

			db.Update(func(tx *bolt.Tx) error {
				tx.CreateBucketIfNotExists(bktName)
				return nil
			})

			db.Update(func(tx *bolt.Tx) error {
				for _, kv := range tc.putV {
					err := putBktValue(tx, bktName, kv.key, kv.value)
					require.Equal(t, tc.err, err)
				}

				return nil
			})

			db.View(func(tx *bolt.Tx) error {
				var dv DepositValue
				require.Nil(t, getBktValue(tx, bktName, tc.key, &dv))
				require.Equal(t, tc.expectV, dv)
				return nil
			})
		})
	}
}

func TestGetBktValue(t *testing.T) {

	type kv struct {
		key   []byte
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
			[]byte("k1"),
			dvs[0],
		},
		{
			[]byte("k2"),
			dvs[1],
		},
		{
			[]byte("k3"),
			dvs[2],
		},
	}

	bktName := []byte("test")

	tt := []struct {
		name    string
		init    []kv
		key     []byte
		v       interface{}
		expectV DepositValue
		err     error
	}{
		{
			"normal",
			init,
			[]byte("k1"),
			&DepositValue{},
			dvs[0],
			nil,
		},
		{
			"not exist",
			init,
			[]byte("k5"),
			&DepositValue{},
			dvs[0],
			fmt.Errorf("value of key %v does not exist in bucket %v", "k5", string(bktName)),
		},
		{
			"invalid accept value",
			init,
			[]byte("k5"),
			DepositValue{},
			dvs[0],
			fmt.Errorf("value is not setable"),
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			db, shutdown := setupDB(t)
			defer shutdown()

			db.Update(func(tx *bolt.Tx) error {
				bkt, err := tx.CreateBucketIfNotExists(bktName)
				require.Nil(t, err)

				for _, kv := range tc.init {
					v, err := json.Marshal(kv.value)
					require.Nil(t, err)
					require.Nil(t, bkt.Put(kv.key, v))
				}

				return nil
			})

			db.View(func(tx *bolt.Tx) error {
				err := getBktValue(tx, bktName, tc.key, tc.v)
				require.Equal(t, tc.err, err)
				if err != nil {
					return err
				}

				v := tc.v.(*DepositValue)

				require.Equal(t, tc.expectV, *v)
				return nil
			})

		})
	}
}

func TestGetCacheHeadDepositValue(t *testing.T) {
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

	tt := []struct {
		name string
		init []DepositValue
		head DepositValue
		ok   bool
	}{
		{
			"normal",
			dvs[:],
			dvs[0],
			true,
		},
		{
			"empty",
			dvs[:0],
			DepositValue{},
			false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			c := newCache()
			for _, dv := range tc.init {
				c.pushDepositValue(dv)
			}

			dv, ok := c.getHeadDepositValue()
			require.Equal(t, tc.ok, ok)
			if ok {
				require.Equal(t, tc.head, dv)
			}
		})
	}

}
