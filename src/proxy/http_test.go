package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"time"

	"bytes"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
	"github.com/stretchr/testify/assert"
)

type gatewayMocker struct {
	err       error
	sleepTime time.Duration
	ack       daemon.Messager
}

func (gw gatewayMocker) AddMonitor(cxt context.Context, m *daemon.MonitorMessage) (ack *daemon.MonitorAckMessage, err error) {
	if gw.err != nil {
		return nil, gw.err
	}
	q := make(chan struct{})
	go func() {
		time.Sleep(gw.sleepTime)
		ack = gw.ack.(*daemon.MonitorAckMessage)
		close(q)
	}()

	select {
	case <-cxt.Done():
		err = cxt.Err()
		return
	case <-q:
		ack = gw.ack.(*daemon.MonitorAckMessage)
	}

	return
}

func (gw gatewayMocker) GetExchangeLogs(cxt context.Context, m *daemon.GetExchangeLogsMessage) (ack *daemon.GetExchangeLogsAckMessage, err error) {
	if gw.err != nil {
		return nil, gw.err
	}
	q := make(chan struct{})
	go func() {
		time.Sleep(gw.sleepTime)
		ack = gw.ack.(*daemon.GetExchangeLogsAckMessage)
		close(q)
	}()

	select {
	case <-cxt.Done():
		err = cxt.Err()
		return
	case <-q:
		ack = gw.ack.(*daemon.GetExchangeLogsAckMessage)
	}

	return
}

func (gw gatewayMocker) Log() logger.Logger {
	return logger.NewLogger("", true)
}

func TestMonitorHandler(t *testing.T) {
	testCases := []struct {
		name          string
		method        string
		target        string
		contentType   string
		code          int
		sleepTime     time.Duration
		req           interface{}
		mockAck       *daemon.MonitorAckMessage
		ack           *daemon.MonitorAckMessage
		addMonitorErr error
	}{
		{
			"Method Not Allowed",
			"GET",
			"/monitor",
			"application/json",
			http.StatusMethodNotAllowed,
			0,
			&daemon.MonitorMessage{},
			nil,
			nil,
			nil,
		},
		{
			"Content-Type Not Acceptable",
			"POST",
			"/monitor",
			"text/plain",
			http.StatusNotAcceptable,
			0,
			&daemon.MonitorMessage{},
			nil,
			nil,
			nil,
		},
		{
			"Request Timeout",
			"POST",
			"/monitor",
			"application/json",
			http.StatusRequestTimeout,
			4 * time.Second,
			&daemon.MonitorMessage{},
			nil,
			nil,
			nil,
		},
		{
			"Internal Sever Error",
			"POST",
			"/monitor",
			"application/json",
			http.StatusInternalServerError,
			0,
			&daemon.MonitorMessage{},
			nil,
			nil,
			errors.New("AddMonitorError"),
		},
		{
			"Bad Request",
			"POST",
			"/monitor",
			"application/json",
			http.StatusBadRequest,
			0,
			"invalid message",
			nil,
			nil,
			nil,
		},
		{
			"200 OK",
			"POST",
			"/monitor",
			"application/json",
			http.StatusOK,
			0,
			&daemon.MonitorMessage{},
			&daemon.MonitorAckMessage{Result: daemon.Result{Success: true}},
			&daemon.MonitorAckMessage{Result: daemon.Result{Success: true}},
			nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// prepare record
			res := httptest.NewRecorder()

			d, err := json.MarshalIndent(tc.req, "", "    ")
			assert.Nil(t, err)

			req := httptest.NewRequest(tc.method, tc.target, bytes.NewBuffer(d))
			defer req.Body.Close()
			// set Content-Type header
			req.Header.Set("Content-Type", tc.contentType)

			MonitorHandler(&gatewayMocker{
				sleepTime: tc.sleepTime,
				err:       tc.addMonitorErr,
				ack:       tc.mockAck,
			})(res, req)

			assert.Equal(t, tc.code, res.Code)
			if res.Code != http.StatusOK {
				return
			}

			var ack daemon.MonitorAckMessage
			err = json.NewDecoder(res.Body).Decode(&ack)
			assert.Nil(t, err)

			assert.Equal(t, *tc.ack, ack)
		})
	}
}

func TestGetExchangeLogsHandle(t *testing.T) {
	testCases := []struct {
		name      string
		method    string
		target    string
		code      int
		sleepTime time.Duration
		ack       *daemon.GetExchangeLogsAckMessage
		mockErr   error
	}{
		{
			"Method Not Allowed",
			"POST",
			"/exchange_logs",
			http.StatusMethodNotAllowed,
			0,
			nil,
			nil,
		},
		{
			"Missing start param",
			"GET",
			"/exchange_logs",
			http.StatusBadRequest,
			0,
			nil,
			nil,
		},
		{
			"Missing end param",
			"GET",
			"/exchange_logs?start=1",
			http.StatusBadRequest,
			0,
			nil,
			nil,
		},
		{
			"Invalid start param",
			"GET",
			"/exchange_logs?start=a",
			http.StatusBadRequest,
			0,
			nil,
			nil,
		},
		{
			"Invalid end param",
			"GET",
			"/exchange_logs?start=1&&end=a",
			http.StatusBadRequest,
			0,
			nil,
			nil,
		},
		{
			"start > end",
			"GET",
			"/exchange_logs?start=2&&end=1",
			http.StatusBadRequest,
			0,
			nil,
			nil,
		},
		{
			"Success",
			"GET",
			"/exchange_logs?start=1&&end=2",
			http.StatusOK,
			0,
			&daemon.GetExchangeLogsAckMessage{
				Base: daemon.Base{
					Id: 1,
				},
				Result: daemon.Result{
					Success: true,
				},
			},
			nil,
		},
		{
			"Failed time out",
			"GET",
			"/exchange_logs?start=1&&end=2",
			http.StatusRequestTimeout,
			4 * time.Second,
			nil,
			nil,
		},
		{
			"Failed get exchange logs failed",
			"GET",
			"/exchange_logs?start=1&&end=2",
			http.StatusInternalServerError,
			0,
			nil,
			errors.New("get exchange logs failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// prepare record
			res := httptest.NewRecorder()
			req, err := http.NewRequest(tc.method, tc.target, nil)
			assert.Nil(t, err)
			ExchangeLogsHandler(&gatewayMocker{
				sleepTime: tc.sleepTime,
				ack:       tc.ack,
				err:       tc.mockErr,
			})(res, req)

			assert.Equal(t, tc.code, res.Code)
			if res.Code != http.StatusOK {
				return
			}

			var ack daemon.GetExchangeLogsAckMessage
			err = json.NewDecoder(res.Body).Decode(&ack)
			assert.Nil(t, err)

			assert.Equal(t, *tc.ack, ack)
		})
	}
}

func TestDivideStartEndRanges(t *testing.T) {
	testCases := []struct {
		blockSize int
		start     int
		end       int
		pairs     [][]int
	}{
		{
			10,
			1,
			11,
			[][]int{
				[]int{1, 10},
				[]int{11, 11},
			},
		},
		{
			10,
			0,
			11,
			[][]int{
				[]int{0, 9},
				[]int{10, 11},
			},
		},
		{
			10,
			0,
			20,
			[][]int{
				[]int{0, 9},
				[]int{10, 19},
				[]int{20, 20},
			},
		},
		{
			10,
			5,
			20,
			[][]int{
				[]int{5, 14},
				[]int{15, 20},
			},
		},
		{
			10,
			5,
			8,
			[][]int{
				[]int{5, 8},
			},
		},
		{
			0,
			5,
			8,
			[][]int{
				[]int{5, 8},
			},
		},
		{
			10,
			8,
			5,
			[][]int{},
		},
	}

	for _, tc := range testCases {
		ps := divideStartEndRange(tc.start, tc.end, tc.blockSize)
		assert.Equal(t, tc.pairs, ps)
	}
}
