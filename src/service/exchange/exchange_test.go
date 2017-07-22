package exchange

// import (
// 	"testing"
// 	"time"

// 	"github.com/skycoin/teller/src/logger"
// 	"github.com/skycoin/teller/src/service/monitor"
// 	"github.com/stretchr/testify/assert"
// )

// func TestCoinValue(t *testing.T) {
// 	db, close := prepareDB(t)
// 	defer close()

// 	bkt := newCoinValueBucket(coinValueBktName, db)
// 	cv := coinValue{
// 		Address:    "a",
// 		Balance:    100,
// 		CoinName:   "btc",
// 		ICOAddress: "a-ico"}
// 	err := bkt.put(cv)
// 	assert.Nil(t, err)

// 	// get the value
// 	cv1, ok := bkt.get("a")
// 	assert.True(t, ok)
// 	assert.Equal(t, cv1, cv)

// 	_, ok = bkt.get("b")
// 	assert.False(t, ok)
// }

// func TestCoinValueBktGetAll(t *testing.T) {
// 	db, close := prepareDB(t)
// 	defer close()
// 	bkt := newCoinValueBucket(coinValueBktName, db)
// 	testData := []struct {
// 		Address string
// 		Balance int64
// 	}{
// 		{
// 			"a",
// 			1,
// 		},
// 	}

// 	for _, d := range testData {
// 		err := bkt.put(coinValue{
// 			Address: d.Address,
// 			Balance: d.Balance,
// 		})
// 		assert.Nil(t, err)
// 	}

// 	balMap, err := bkt.getAllBalances()
// 	assert.Nil(t, err)
// 	for _, d := range testData {
// 		v, ok := balMap[d.Address]
// 		assert.True(t, ok)
// 		assert.Equal(t, d.Balance, v)
// 	}
// }

// func TestCoinValueBucketChangeBalance(t *testing.T) {
// 	db, close := prepareDB(t)
// 	defer close()
// 	bkt := newCoinValueBucket(coinValueBktName, db)
// 	bkt.put(coinValue{
// 		Address: "a",
// 		Balance: 1,
// 	})

// 	err := bkt.changeBalance("a", 2)
// 	assert.Nil(t, err)
// 	cv, ok := bkt.get("a")
// 	assert.True(t, ok)
// 	assert.Equal(t, int64(3), cv.Balance)
// }

// func TestEventHandler(t *testing.T) {
// 	db, close := prepareDB(t)
// 	defer close()
// 	cfg := exchgConfig{
// 		db:  db,
// 		log: logger.NewLogger("", true),
// 		rateTable: rateTable{
// 			[]struct {
// 				Date time.Time
// 				Rate float64
// 			}{
// 				{
// 					time.Time{},
// 					1,
// 				},
// 			},
// 		},

// 		checkPeriod: 5 * time.Second,
// 		nodeRPCAddr: "127.0.0.1:7430",
// 		nodeWltFile: "/Users/heaven/.skycoin/wallets/skycoin.wlt",
// 	}

// 	ex := newExchange(&cfg)
// 	ex.coinValue.put(coinValue{
// 		Address:    "a",
// 		Balance:    0,
// 		ICOAddress: "cBB6zToAjhMkvRrWaz8Tkv6QZxjKCfJWBx",
// 	})

// 	err := ex.eventHandler(monitor.Event{
// 		Type: monitor.EBalanceChange,
// 		Value: monitor.AddressValue{
// 			Address: "a",
// 			Value:   1 * 10e9,
// 		},
// 	})
// 	assert.Nil(t, err)
// }

// func TestRateTable(t *testing.T) {
// 	now := time.Now()
// 	testCases := []struct {
// 		name       string
// 		tb         rateTable
// 		expectRate float64
// 		ok         bool
// 	}{
// 		{
// 			"get 10",
// 			rateTable{
// 				rates: []struct {
// 					Date time.Time
// 					Rate float64
// 				}{
// 					{
// 						now,
// 						10,
// 					},
// 					{
// 						now.Add(1000),
// 						20,
// 					},
// 					{
// 						now.Add(2000),
// 						30,
// 					},
// 				},
// 			},
// 			10,
// 			true,
// 		},
// 		{
// 			"get 20",
// 			rateTable{
// 				rates: []struct {
// 					Date time.Time
// 					Rate float64
// 				}{
// 					{
// 						now.Add(-1000),
// 						10,
// 					},
// 					{
// 						now,
// 						20,
// 					},
// 					{
// 						now.Add(2000),
// 						30,
// 					},
// 				},
// 			},
// 			20,
// 			true,
// 		},
// 		{
// 			"get zero",
// 			rateTable{
// 				rates: []struct {
// 					Date time.Time
// 					Rate float64
// 				}{},
// 			},
// 			0.0,
// 			false,
// 		},
// 		{
// 			"get basic 10",
// 			rateTable{
// 				rates: []struct {
// 					Date time.Time
// 					Rate float64
// 				}{
// 					{
// 						time.Time{},
// 						10,
// 					},
// 					{
// 						now.Add(1000),
// 						20,
// 					},
// 					{
// 						now.Add(2000),
// 						30,
// 					},
// 				},
// 			},
// 			10,
// 			true,
// 		},
// 	}

// 	for _, tc := range testCases {
// 		t.Run(tc.name, func(t *testing.T) {
// 			rate, ok := tc.tb.get(now)
// 			assert.Equal(t, tc.ok, ok)
// 			if ok {
// 				assert.Equal(t, tc.expectRate, rate)
// 			}
// 		})
// 	}
// }
