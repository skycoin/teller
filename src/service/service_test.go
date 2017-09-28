package service

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"testing"

	"os"

	"time"

	"github.com/boltdb/bolt"
	"github.com/fortytw2/leaktest"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
	"github.com/stretchr/testify/assert"
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
	assert.Nil(t, err)
	var s *daemon.Session
	var wg sync.WaitGroup
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mux := daemon.NewMux(logger.NewLogger("", true))
			for tp, hd := range msgHandler {
				mux.HandleFunc(tp, hd)
			}

			s, err = daemon.NewSession(conn, auth, mux, false)
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.Run()
			}()
		}
	}()

	return ln, func() {
		if s != nil {
			s.Close()
		}
		ln.Close()
		wg.Wait()
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

func setupDB(t *testing.T) (*bolt.DB, func()) {
	rand.Seed(time.Now().Unix())
	f := fmt.Sprintf("%s/test%d.db", os.TempDir(), rand.Intn(1024))
	db, err := bolt.Open(f, 0700, nil)
	assert.Nil(t, err)
	return db, func() {
		db.Close()
		os.Remove(f)
	}
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

func (de dummyExchanger) BindNum(skyAddr string) int {
	if de.skyAddrs == nil {
		return 0
	}

	return len(de.skyAddrs[skyAddr])
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
			defer leaktest.Check(t)()
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

			service.Run()
		})
	}

}
