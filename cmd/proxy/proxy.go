package main

import (
	"os"
	"os/signal"
	"sync"

	"flag"

	"github.com/google/gops/agent"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
	"github.com/skycoin/teller/src/proxy"
)

func main() {
	proxyAddr := flag.String("teller-proxy-addr", "0.0.0.0:7070", "teller proxy listen address")
	httpAddr := flag.String("http-service-addr", "localhost:7071", "http api service address")
	flag.Parse()

	log := logger.NewLogger("", false)

	// start gops agent, for profilling
	if err := agent.Listen(&agent.Options{
		NoShutdownCleanup: true,
	}); err != nil {
		log.Println("Start profile agent failed:", err)
		return
	}

	_, lseckey := cipher.GenerateKeyPair()
	log.Println("Pubkey:", cipher.PubKeyFromSecKey(lseckey).Hex())

	auth := &daemon.Auth{
		LSeckey: lseckey,
	}

	px := proxy.New(*proxyAddr, *httpAddr, auth, proxy.Logger(log))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		px.Run()
	}()

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan
	signal.Stop(sigchan)
	log.Println("Shutting down...")

	px.Shutdown()
	wg.Wait()
	log.Println("Shutdown complete")
}
