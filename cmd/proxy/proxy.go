package main

import (
	"log"
	"os"
	"os/signal"

	"flag"

	"github.com/google/gops/agent"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
	"github.com/skycoin/teller/src/proxy"
)

const (
	RemotePubkey = "02c3de61a2cb5c055ff09c8c40277c9826ca8a1a4d2a5faaccd318079a6224d851"
	LocalSeckey  = "f7543361c75aa1e9e8cdbe703437197745d22019db7e53ff6f668d5ddf9eb7f1"
)

func main() {
	proxyAddr := flag.String("proxy-addr", "0.0.0.0:7070", "proxy listen address")
	httpAddr := flag.String("http-service-addr", "localhost:7071", "http api service address")
	flag.Parse()

	// start gops agent, for profilling
	if err := agent.Listen(&agent.Options{
		NoShutdownCleanup: true,
	}); err != nil {
		log.Fatal(err)
	}

	rpubkey := cipher.MustPubKeyFromHex(RemotePubkey)
	lseckey := cipher.MustSecKeyFromHex(LocalSeckey)

	auth := &daemon.Auth{
		RPubkey: rpubkey,
		LSeckey: lseckey,
	}

	log := logger.NewLogger("", true)
	px, close := proxy.New(*proxyAddr, *httpAddr, auth, proxy.Logger(log))
	px.Run()

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan
	signal.Stop(sigchan)
	close()
}
