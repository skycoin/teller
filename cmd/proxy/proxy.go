package main

import (
	"os"
	"os/signal"
	"os/user"
	"sync"
	"time"

	"flag"

	"path/filepath"

	"github.com/google/gops/agent"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
	"github.com/skycoin/teller/src/proxy"
)

func main() {
	proxyAddr := flag.String("teller-proxy-addr", "0.0.0.0:7070", "teller proxy listen address")
	httpAddr := flag.String("http-service-addr", "localhost:7071", "http api service address")
	httpsAddr := flag.String("https-service-addr", "", "https api service address")
	autoTlsHost := flag.String("auto-tls-host", "", "generate certificate with Let's Encrypt for this hostname and use it")
	tlsKey := flag.String("tls-key", "", "tls key file (if not using -auto-tls-host)")
	tlsCert := flag.String("tls-cert", "", "tls cert file (if not using -auto-tls-host)")
	htmlInterface := flag.Bool("html", true, "serve html interface")
	htmlStaticDir := flag.String("html-static-dir", "./web/build", "static directory to serve html interface from")
	startAt := flag.String("start-time", "", "Don't process API requests until after this timestamp (RFC3339 format)")
	debug := flag.Bool("debug", false, "debug mode will show more logs")
	flag.Parse()

	log := logger.NewLogger("", *debug)

	// start gops agent, for profilling
	if err := agent.Listen(&agent.Options{
		NoShutdownCleanup: true,
	}); err != nil {
		log.Println("Start profile agent failed:", err)
		return
	}

	_, lseckey := cipher.GenerateKeyPair()
	pubkey := cipher.PubKeyFromSecKey(lseckey).Hex()
	log.Println("Pubkey:", pubkey)

	auth := &daemon.Auth{
		LSeckey: lseckey,
	}

	startAtStamp := time.Time{}
	if *startAt != "" {
		var err error
		startAtStamp, err = time.Parse(time.RFC3339, *startAt)
		if err != nil {
			log.Fatalln("Invalid -start-time, must be in RCF3339 format", time.RFC3339)
		}
		startAtStamp = startAtStamp.UTC()
	}

	pxCfg := proxy.ProxyConfig{
		SrvAddr:       *proxyAddr,
		HttpSrvAddr:   *httpAddr,
		HtmlStaticDir: *htmlStaticDir,
		HtmlInterface: *htmlInterface,
		HttpsSrvAddr:  *httpsAddr,
		StartAt:       startAtStamp,
		AutoTlsHost:   *autoTlsHost,
		TlsCert:       *tlsCert,
		TlsKey:        *tlsKey,
	}

	px := proxy.New(pxCfg, auth, proxy.Logger(log))

	var wg sync.WaitGroup
	wg.Add(1)
	errC := make(chan error, 1)

	go func() {
		defer wg.Done()
		errC <- px.Run()
	}()

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)

	select {
	case <-sigchan:
	case err := <-errC:
		if err != nil {
			log.Println("ERROR:", err)
		}
	}

	signal.Stop(sigchan)
	log.Println("Shutting down...")

	px.Shutdown()
	wg.Wait()
	log.Println("Shutdown complete")
}

func createAppDirIfNotExist(app string) (string, error) {
	cur, err := user.Current()
	if err != nil {
		return "", err
	}
	path := filepath.Join(cur.HomeDir, app)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// create the dir
		if err := os.Mkdir(path, 0700); err != nil {
			return "", err
		}
	}
	return path, nil
}
