package service

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/fortytw2/leaktest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/util/logger"
)

func pingHandler(w daemon.ResponseWriteCloser, msg daemon.Messager) {
	switch msg.Type() {
	case daemon.PingMessage{}.Type():
		w.Write(&daemon.PongMessage{Value: "PONG"})
	default:
		fmt.Println("unknow message type:", msg.Type())
	}
}

func makeProxy(t *testing.T, auth *daemon.Auth, msgHandler map[daemon.MsgType]daemon.Handler) (net.Listener, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var s *daemon.Session
	var wg sync.WaitGroup

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				t.Log(err)
				return
			}

			mux := daemon.NewMux(logger.NewLogger("", true))
			for tp, hd := range msgHandler {
				mux.HandleFunc(tp, hd)
			}

			s, err = daemon.NewSession(conn, auth, mux, false)
			assert.NoError(t, err)
			if err != nil {
				return
			}

			wg.Add(1)

			go func() {
				defer wg.Done()
				err := s.Run()
				if err != nil {
					t.Log(err)
				}
			}()
		}
	}()

	return ln, func() {
		if s != nil {
			t.Logf("Closing daemon.Session")
			s.Close()
			wg.Wait()
		}

		err := ln.Close()
		assert.NoError(t, err)
	}
}

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

type dummyExchanger struct {
	err      error
	skyAddrs map[string][]string
}

func (de dummyExchanger) BindAddress(btcAddr, skyAddr string) error {
	if de.err != nil {
		return de.err
	}

	if de.skyAddrs == nil {
		de.skyAddrs = make(map[string][]string)
	}

	btcAddrs := de.skyAddrs[skyAddr]
	if btcAddrs == nil {
		btcAddrs = []string{}
	}

	btcAddrs = append(btcAddrs, btcAddr)
	de.skyAddrs[skyAddr] = btcAddrs

	return de.err
}

func (de dummyExchanger) GetDepositStatuses(skyAddr string) ([]daemon.DepositStatus, error) {
	return nil, nil
}

func (de dummyExchanger) BindNum(skyAddr string) (int, error) {
	if de.skyAddrs == nil {
		return 0, nil
	}

	return len(de.skyAddrs[skyAddr]), nil
}

type dummyBtcAddrGenerator struct {
	addr string
	err  error
}

func (dba dummyBtcAddrGenerator) NewAddress() (string, error) {
	return dba.addr, dba.err
}

func TestRunService(t *testing.T) {
	var testCases = []struct {
		name               string
		proxyMsgHanlderMap map[daemon.MsgType]daemon.Handler
		exchanger          Exchanger
		btcAddrGen         BtcAddrGenerator
		shutdownTime       time.Duration
	}{
		{
			"test_ping_pong",
			map[daemon.MsgType]daemon.Handler{
				daemon.PingMsgType: pingHandler,
			},
			&dummyExchanger{},
			dummyBtcAddrGenerator{addr: "1JNonvXRyZvZ4ZJ9PE8voyo67UQN1TpoGy"},
			time.Second * 5,
		},
		{
			"pong_timeout",
			map[daemon.MsgType]daemon.Handler{
				daemon.PingMsgType: func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
					time.Sleep(pongTimeout + time.Second)
				},
			},
			&dummyExchanger{},
			dummyBtcAddrGenerator{addr: "1JNonvXRyZvZ4ZJ9PE8voyo67UQN1TpoGy"},
			pongTimeout + time.Second,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := context.WithTimeout(context.Background(), tc.shutdownTime+time.Second)
			defer leaktest.CheckContext(ctx, t)()

			cAuth, sAuth := makeAuthPair()
			s, stop := makeProxy(t, sAuth, tc.proxyMsgHanlderMap)
			defer stop()
			proxyAddr := s.Addr().String()

			lg := logger.NewLogger("", true)

			service := New(Config{
				ProxyAddr: proxyAddr,
			}, cAuth, lg, tc.exchanger, tc.btcAddrGen)

			service.HandleFunc(daemon.PongMessage{}.Type(), PongMessageHandler(service.gateway))

			time.AfterFunc(tc.shutdownTime, func() {
				service.Shutdown()
			})

			err := service.Run()
			require.NoError(t, err)
		})
	}
}
