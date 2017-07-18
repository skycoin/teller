package service

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"testing"

	"time"

	"os"

	"github.com/boltdb/bolt"
	"github.com/fortytw2/leaktest"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
	"github.com/stretchr/testify/assert"
)

func makeServer(t *testing.T, auth *daemon.Auth) (net.Listener, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	assert.Nil(t, err)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mux := daemon.NewMux(logger.NewLogger("", true))
			mux.HandleFunc(daemon.PingMessage{}.Type(), func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
				switch msg.Type() {
				case daemon.PingMessage{}.Type():
					fmt.Println("recv ping message")
					w.Write(&daemon.PongMessage{Value: "PONG"})
				default:
					fmt.Println("unknow message type:", msg.Type())
				}
			})
			s, err := daemon.NewSession(conn, auth, mux, false)
			if err != nil {
				return
			}
			cxt, cancel := context.WithCancel(context.Background())
			defer cancel()
			go s.Run(cxt)
		}
	}()
	return ln, func() {
		ln.Close()
		fmt.Println("close listener")
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

func prepareDB(t *testing.T) (*bolt.DB, func()) {
	f := fmt.Sprintf("test%d.db", rand.Intn(1024))
	db, err := bolt.Open(f, 0700, nil)
	assert.Nil(t, err)
	return db, func() {
		db.Close()
		os.Remove(f)
	}
}

func TestRunService(t *testing.T) {
	defer leaktest.Check(t)()
	cAuth, sAuth := makeAuthPair()
	s, stop := makeServer(t, sAuth)
	defer stop()

	proxyAddr := s.Addr().String()

	lg := logger.NewLogger("", true)
	db, close := prepareDB(t)
	defer close()
	service, cancel := New(proxyAddr, cAuth, db, Logger(lg))

	service.HandleFunc(daemon.PongMessage{}.Type(), PongMessageHandler(service.gateway))

	// time.AfterFunc(15*time.Second, func() {
	// 	cancel()
	// })
	service.Run()
	time.Sleep(5 * time.Second)
	cancel()
	// time.Sleep(3 * time.Second)
}
