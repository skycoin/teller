package service

import (
	"testing"
	"time"

	"github.com/skycoin/teller/src/daemon"
	"github.com/stretchr/testify/assert"
)

func TestExchangeLogPut(t *testing.T) {
	logs := makeExchangeLogs(2)
	db, close := prepareDB(t)
	defer close()
	bkt := newExchangeLogBucket(exchangeLogBktName, db)
	for _, log := range logs {
		bkt.put(&log)
	}

	assert.Equal(t, 2, bkt.len())
}

func TestExchangeLogGet(t *testing.T) {
	logs := makeExchangeLogs(10)

	db, close := prepareDB(t)
	defer close()
	bkt := newExchangeLogBucket(exchangeLogBktName, db)
	for i, log := range logs {
		bkt.put(&log)
		logs[i].ID = log.ID
	}
	assert.Equal(t, 10, bkt.len())

	testCases := []struct {
		name   string
		init   []daemon.ExchangeLog
		start  int
		end    int
		expect []daemon.ExchangeLog
	}{
		{
			"start from 0",
			logs,
			0,
			1,
			[]daemon.ExchangeLog{logs[0]},
		},
		{
			"start from 0 with 2 value",
			logs,
			0,
			2,
			[]daemon.ExchangeLog{logs[0], logs[1]},
		},
		{
			"start from 1 with 2 value",
			logs,
			1,
			2,
			[]daemon.ExchangeLog{logs[0], logs[1]},
		},
		{
			"start from 2 with 2 value",
			logs,
			2,
			3,
			[]daemon.ExchangeLog{logs[1], logs[2]},
		},
		{
			"start from 5 with 6 value",
			logs,
			5,
			10,
			[]daemon.ExchangeLog{logs[4], logs[5], logs[6], logs[7], logs[8], logs[9]},
		},
		{
			"end beyond len",
			logs,
			7,
			12,
			[]daemon.ExchangeLog{logs[6], logs[7], logs[8], logs[9]},
		},
	}

	for _, tc := range testCases {
		lgs, err := bkt.get(tc.start, tc.end)
		assert.Nil(t, err)
		assert.Equal(t, tc.expect, lgs)
	}

}

func makeExchangeLogs(n int) (logs []daemon.ExchangeLog) {
	now := time.Now()
	logs = make([]daemon.ExchangeLog, n)
	for i := 0; i < n; i++ {
		log := daemon.ExchangeLog{
			Time: now.Add(time.Duration(i) * 100),
			Deposit: struct {
				Address string
				Coin    uint64
			}{
				"a",
				100 * (uint64(i) + 1),
			},
			ICO: struct {
				Address string
				Coin    uint64
			}{
				"b",
				100 * 100 * (uint64(i) + 1),
			},
		}
		logs[i] = log
	}
	return
}

func TestExchangeLogsUpdate(t *testing.T) {
	logs := makeExchangeLogs(1)

	db, close := prepareDB(t)
	defer close()
	bkt := newExchangeLogBucket(exchangeLogBktName, db)
	for i, log := range logs {
		bkt.put(&log)
		logs[i].ID = log.ID
	}
	assert.Equal(t, 1, bkt.len())

	elaps := 100
	err := bkt.update(logs[0].ID, func(log *daemon.ExchangeLog) {
		log.Time = logs[0].Time.Add(time.Duration(elaps))
		log.Tx.Confirmed = true
	})
	assert.Nil(t, err)

	glogs, err := bkt.get(logs[0].ID, logs[0].ID)
	assert.Nil(t, err)
	assert.Equal(t, glogs[0].Time, logs[0].Time.Add(time.Duration(elaps)))
	assert.Equal(t, glogs[0].Tx.Confirmed, true)
}
