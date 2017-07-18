package daemon

import (
	"net"
	"testing"

	"github.com/skycoin/skycoin/src/cipher"
	"github.com/stretchr/testify/assert"
)

func makeTestConn(t *testing.T) (client, server net.Conn) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	assert.Nil(t, err)
	c := make(chan struct{})
	go func() {
		defer close(c)
		defer ln.Close()
		server, err = ln.Accept()
		assert.Nil(t, err)
	}()

	client, err = net.Dial("tcp", ln.Addr().String())
	assert.Nil(t, err)
	<-c
	return
}

func TestTransportHandshake(t *testing.T) {
	c, s := makeTestConn(t)
	_, csecKey := cipher.GenerateKeyPair()
	// spubKey, _ := cipher.GenerateKeyPair()
	spubKey, ssecKey := cipher.GenerateKeyPair()
	go func() {
		_, err := newTransport(s, &Auth{
			LSeckey: ssecKey,
		}, false)
		assert.Nil(t, err)
	}()

	_, err := newTransport(c, &Auth{
		LSeckey: csecKey,
		RPubkey: spubKey,
	}, true)
	assert.Nil(t, err)
}
