package monitor

import (
	"context"
	"testing"

	"time"

	"github.com/stretchr/testify/assert"
)

type BalancesMock struct {
	Local    map[string]int64
	Realtime map[string]int64
}

func (b BalancesMock) GetAllLocalBalances() map[string]int64 {
	rlt := make(map[string]int64, len(b.Local))
	for k, v := range b.Local {
		rlt[k] = v
	}
	return rlt
}

func (b BalancesMock) GetRealtimeBalance(addr string) (int64, error) {
	v := b.Realtime[addr]
	return v, nil
}

func TestNewMonitor(t *testing.T) {
	t.Run("default configs", func(t *testing.T) {
		m := New("btc", &BalancesMock{})
		assert.Equal(t, defaultCheckPeriod, m.checkPeriod)
		assert.Equal(t, defaultEventBufferSize, m.eventBuffSize)
		assert.Equal(t, defaultPushTimeout, m.pushTimeout)
	})

	t.Run("config through option", func(t *testing.T) {
		testData := []struct {
			EventBuffSize int
			CheckPeriod   time.Duration
			PushTimeout   time.Duration
		}{
			{
				1,
				2,
				3,
			},
			{
				2,
				3,
				3,
			},
		}

		for _, d := range testData {
			m := New("btc",
				&BalancesMock{},
				EventBuffSize(d.EventBuffSize),
				PushTimeout(d.PushTimeout),
				CheckPeriod(d.CheckPeriod))

			assert.Equal(t, d.CheckPeriod, m.checkPeriod)
			assert.Equal(t, d.EventBuffSize, m.eventBuffSize)
			assert.Equal(t, d.PushTimeout, m.pushTimeout)
		}
	})
}

func TestMonitor(t *testing.T) {
	testData := []struct {
		Name         string
		Local        map[string]int64
		Realtime     map[string]int64
		MonitorAddrs []AddressValue
	}{
		{
			"got one change",
			map[string]int64{"a": 0},
			map[string]int64{"a": 1},
			[]AddressValue{
				{
					"a",
					1,
				},
			},
		},
		{
			"got change two",
			map[string]int64{"a": 0},
			map[string]int64{"a": 2},
			[]AddressValue{
				{
					"a",
					2,
				},
			},
		},
	}
	for _, d := range testData {
		t.Run(d.Name, func(t *testing.T) {
			bm := &BalancesMock{
				Local:    d.Local,
				Realtime: d.Realtime,
			}

			cxt, cancel := context.WithCancel(context.Background())
			defer cancel()
			monitor := New("btc", bm, CheckPeriod(1*time.Second))
			ec := monitor.Run(cxt)

			event := <-ec
			e := Event{
				Type: EBalanceChange,
				Value: AddressValue{
					d.MonitorAddrs[0].Address,
					d.MonitorAddrs[0].Value,
				},
			}

			assert.Equal(t, e, event)
		})
	}
}

func TestAddAddress(t *testing.T) {
	bm := &BalancesMock{
		Local:    map[string]int64{"a": 0},
		Realtime: map[string]int64{"a": 1},
	}
	cxt, cancel := context.WithCancel(context.Background())
	defer cancel()
	monitor := New("btc", bm, CheckPeriod(1*time.Second))
	monitor.Run(cxt)
	monitor.AddAddress("b")
	time.Sleep(1 * time.Second)
	_, ok := monitor.balances["b"]
	assert.True(t, ok)
}
