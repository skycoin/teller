package service

import (
	"bytes"
	"errors"
	"testing"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
	"github.com/stretchr/testify/assert"
)

type GatewayMocker struct {
	resetPongTimer bool
	mm             map[string]error
	exchangeLogs   []daemon.ExchangeLog
	logger         logger.Logger
}

func (gate *GatewayMocker) Logger() logger.Logger {
	return gate.logger

}

func (gate *GatewayMocker) ResetPongTimer() {
	gate.resetPongTimer = true
}

func (gate *GatewayMocker) AddMonitor(m *daemon.MonitorMessage) error {
	return gate.mm[m.DepositCoin.Address]
}

func (gate *GatewayMocker) GetExchangeLogs(start, end int) ([]daemon.ExchangeLog, error) {
	if start > end {
		return []daemon.ExchangeLog{}, errors.New("Get exchange logs failed, err: start must <= end")
	}
	return gate.exchangeLogs[:], nil
}

func (gate *GatewayMocker) GetExchangeLogsLen() int {
	return len(gate.exchangeLogs)
}

type ResWC struct {
	ackMsg daemon.Messager
	closed bool
	write  bool
}

func (wc *ResWC) Write(m daemon.Messager) {
	wc.write = true
	wc.ackMsg = m
}

func (wc *ResWC) Close() {
	wc.closed = true
}

func TestMonitorMessageHandler(t *testing.T) {
	testCases := []struct {
		name   string
		gwMap  map[string]error // use is used to control whether return error when call AddMonitor function
		req    *daemon.MonitorMessage
		ack    *daemon.MonitorAckMessage
		closed bool
	}{
		{
			"add monitor success",
			map[string]error{
				"a": nil,
			},
			&daemon.MonitorMessage{
				DepositCoin: struct {
					CoinName string `json:"coin"`
					Address  string `json:"address"`
				}{
					"BTC",
					"a",
				},
			},
			&daemon.MonitorAckMessage{
				Result: daemon.Result{Success: true},
			},
			false,
		},
		{
			"add monitor failed",
			map[string]error{
				"a": errors.New("add monitor failed"),
			},
			&daemon.MonitorMessage{
				DepositCoin: struct {
					CoinName string `json:"coin"`
					Address  string `json:"address"`
				}{
					"BTC",
					"a",
				},
			},
			&daemon.MonitorAckMessage{
				Result: daemon.Result{Success: false, Err: "add monitor failed"},
			},
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hd := MonitorMessageHandler(&GatewayMocker{
				logger: logger.NewLogger("test", true),
				mm:     tc.gwMap,
			})
			wc := &ResWC{}
			hd(wc, tc.req)
			ack, ok := wc.ackMsg.(*daemon.MonitorAckMessage)
			assert.True(t, ok)
			assert.Equal(t, *tc.ack, *ack)
			assert.Equal(t, tc.closed, wc.closed)
		})
	}
}

func TestGetExchangeLogsHandler(t *testing.T) {
	logs := makeExchangeLogs(10)
	testCases := []struct {
		name         string
		logs         []daemon.ExchangeLog
		req          daemon.Messager
		res          daemon.Messager
		writeSuccess bool
		errLogStr    string
	}{
		{
			"get one logs",
			[]daemon.ExchangeLog{logs[0]},
			&daemon.GetExchangeLogsMessage{
				Base: daemon.Base{
					Id: 1,
				},
				StartID: 0,
				EndID:   1,
			},
			&daemon.GetExchangeLogsAckMessage{
				Base: daemon.Base{
					Id: 1,
				},
				Result: daemon.Result{
					Success: true,
				},
				MaxLogID: 1,
				Logs: []daemon.ExchangeLog{
					logs[0],
				},
			},
			true,
			"",
		},
		{
			"nil GetExchangeLogsMessage",
			[]daemon.ExchangeLog{logs[0]},
			nil,
			nil,
			false,
			"Request message is nil",
		},
		{
			"invalid GetExchangeLogsMessage",
			[]daemon.ExchangeLog{logs[0]},
			&daemon.PingMessage{},
			nil,
			false,
			"Expect GLOG message, but got:PING",
		},
		{
			"Invalid logs id range",
			[]daemon.ExchangeLog{logs[0]},
			&daemon.GetExchangeLogsMessage{
				Base: daemon.Base{
					Id: 1,
				},
				StartID: 2,
				EndID:   1,
			},
			&daemon.GetExchangeLogsAckMessage{
				Base: daemon.Base{
					Id: 1,
				},
				Result: daemon.Result{
					Success: false,
					Err:     "Get exchange logs failed, err: start must <= end",
				},
			},
			true,
			"Get exchange logs failed, err: Get exchange logs failed, err: start must <= end",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			writer := bytes.Buffer{}
			gw := &GatewayMocker{
				exchangeLogs: tc.logs,
				logger:       logger.NewLogger("test", true, &writer),
			}
			hd := GetExchangeLogsHandler(gw)
			wc := &ResWC{}
			hd(wc, tc.req)
			assert.Equal(t, tc.writeSuccess, wc.write)
			if tc.writeSuccess {
				ack, ok := wc.ackMsg.(*daemon.GetExchangeLogsAckMessage)
				assert.True(t, ok)
				assert.Equal(t, tc.res, ack)
				return
			}
			assert.Contains(t, writer.String(), tc.errLogStr)
		})
	}
}
