package proxy

import (
	"context"
	"errors"
	"net"
	"testing"

	"time"

	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
	"github.com/stretchr/testify/assert"
)

func makeAuthPair() (clientAuth, serverAuth *daemon.Auth) {
	spubkey, sseckey := cipher.GenerateKeyPair()
	cpubkey, cseckey := cipher.GenerateKeyPair()
	clientAuth = &daemon.Auth{
		LSeckey: cseckey,
		RPubkey: spubkey,
	}

	serverAuth = &daemon.Auth{
		LSeckey: sseckey,
		RPubkey: cpubkey,
	}
	return
}

func TestAddMonitor(t *testing.T) {
	testCases := []struct {
		name           string
		monitorHandler daemon.Handler
		err            error
		ackSucess      bool
		subNum         int
	}{
		{
			"Monitor Success",
			func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
				w.Write(&daemon.MonitorAckMessage{
					Base: daemon.Base{
						Id: msg.ID(),
					},
					Result: daemon.Result{Success: true}})
			},
			nil,
			true,
			0,
		},
		{
			"Monitor Timeout",
			func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
			},
			context.DeadlineExceeded,
			false,
			0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ca, sa := makeAuthPair()
			// start the proxy
			log := logger.NewLogger("", true)
			proxy, stopProxy := New("127.0.0.1:0", "127.0.0.1:0", sa, Logger(log))
			defer stopProxy()
			go proxy.Run()

			time.Sleep(1 * time.Second)
			// get proxy serve address
			proxyAddr := proxy.ln.Addr().String()

			// create gateway
			gw := &gateway{proxy}

			// start the ico-service client
			conn, err := net.Dial("tcp", proxyAddr)
			assert.Nil(t, err)

			mux := daemon.NewMux(log)
			mux.HandleFunc(daemon.MonitorMsgType, tc.monitorHandler)
			cltS, err := daemon.NewSession(conn, ca, mux, true, daemon.Logger(log))
			assert.Nil(t, err)
			cxt, cancel := context.WithCancel(context.Background())
			defer cancel()
			go cltS.Run(cxt)
			time.Sleep(1 * time.Second)

			cxt, cl := context.WithTimeout(context.Background(), 3*time.Second)
			defer cl()
			ack, err := gw.AddMonitor(cxt, &daemon.MonitorMessage{})
			assert.Equal(t, tc.err, err)
			if err != nil {
				return
			}
			assert.True(t, tc.ackSucess, ack.Success)
		})
	}
}

func TestGetExchangeLogs(t *testing.T) {
	testCases := []struct {
		name      string
		handler   daemon.Handler
		err       error
		ackSucess bool
	}{
		{
			"Get logs success",
			func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
				w.Write(&daemon.GetExchangeLogsAckMessage{
					Base: daemon.Base{
						Id: msg.ID(),
					},
					Result: daemon.Result{Success: true}})
			},
			nil,
			true,
		},
		{
			"Get exchange logs timeout",
			func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
			},
			context.DeadlineExceeded,
			false,
		},
		{
			"Invalid ack message",
			func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
				w.Write(&daemon.PingMessage{
					Base: daemon.Base{
						Id: msg.ID(),
					},
					Value: "PING",
				})
			},
			errors.New("Can't assign value of type:*daemon.PingMessage to *daemon.GetExchangeLogsAckMessage"),
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ca, sa := makeAuthPair()
			// start the proxy
			log := logger.NewLogger("", true)
			proxy, stopProxy := New("127.0.0.1:0", "127.0.0.1:0", sa, Logger(log))
			defer stopProxy()
			go proxy.Run()

			time.Sleep(1 * time.Second)
			// get proxy serve address
			proxyAddr := proxy.ln.Addr().String()

			// create gateway
			gw := &gateway{proxy}

			// start the ico-service client
			conn, err := net.Dial("tcp", proxyAddr)
			assert.Nil(t, err)

			mux := daemon.NewMux(log)
			mux.HandleFunc(daemon.GetExchgLogsMsgType, tc.handler)
			cltS, err := daemon.NewSession(conn, ca, mux, true, daemon.Logger(log))
			assert.Nil(t, err)
			cxt, cancel := context.WithCancel(context.Background())
			defer cancel()
			go cltS.Run(cxt)
			time.Sleep(1 * time.Second)

			cxt, cl := context.WithTimeout(cxt, 3*time.Second)
			defer cl()
			ack, err := gw.GetExchangeLogs(cxt, &daemon.GetExchangeLogsMessage{})
			assert.Equal(t, tc.err, err)
			if err != nil {
				return
			}
			assert.True(t, tc.ackSucess, ack.Success)
		})
	}

}
