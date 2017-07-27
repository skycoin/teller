// Skycoin teller, which provides service of monitoring the bitcoin deposite
// and sending skycoin coins
package main

import (
	"io/ioutil"
	"log"
	"os/signal"
	"os/user"
	"sync"
	"time"

	"path/filepath"

	"github.com/boltdb/bolt"
	"github.com/google/gops/agent"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
	"github.com/skycoin/teller/src/service"
	"github.com/skycoin/teller/src/service/btcaddrs"
	"github.com/skycoin/teller/src/service/cli"
	"github.com/skycoin/teller/src/service/config"
	"github.com/skycoin/teller/src/service/scanner"

	"fmt"
	"os"

	"bytes"

	"github.com/btcsuite/btcrpcclient"
	"github.com/skycoin/teller/src/service/exchange"
	"github.com/skycoin/teller/src/service/sender"
)

const (
	RemotePubkey = "03554c1787c0d49ddbced3b9d4f9f1163c01f5d1bcf52ae7362a63027e1a896cef"
	LocalSeckey  = "81ea5dbfa6fab837bbcb96480cff49b57e7135a15e7d0a6d021177edecf246d3"

	appDir = ".skycoin-teller"
	dbName = "data.db"
)

func main() {
	// generate auth key pairs
	// for i := 0; i < 2; i++ {
	// 	pub, sec := cipher.GenerateKeyPair()
	// 	fmt.Println(pub.Hex())
	// 	fmt.Println(sec.Hex())
	// }

	// start gops agent, for profilling
	if err := agent.Listen(&agent.Options{
		NoShutdownCleanup: true,
	}); err != nil {
		log.Fatal(err)
	}

	// init logger
	log := logger.NewLogger("", false)

	quit := make(chan struct{})
	go catchInterrupt(quit)

	// load config
	cfg, err := config.New("config.json")
	if err != nil {
		log.Println("Load config failed:", err)
		return
	}

	rpubkey := cipher.MustPubKeyFromHex(RemotePubkey)
	lseckey := cipher.MustSecKeyFromHex(LocalSeckey)

	appDir, err := createAppDirIfNotExist(appDir)
	if err != nil {
		log.Println("Create AppDir failed:", err)
		return
	}

	// open db
	dbPath := filepath.Join(appDir, dbName)
	db, err := bolt.Open(dbPath, 0700, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Printf("Open db failed, err: %v\n", err)
		return
	}

	// prepare auth
	auth := &daemon.Auth{
		RPubkey: rpubkey,
		LSeckey: lseckey,
	}

	// create btc rpc client
	btcrpcConnConf := makeBtcrpcConfg(*cfg)
	btcrpc, err := btcrpcclient.New(&btcrpcConnConf, nil)
	if err != nil {
		log.Printf("Connect btcd failed: %v\n", err)
		return
	}

	log.Println("Connect to btcd success")

	errC := make(chan error, 10)
	wg := sync.WaitGroup{}

	// create scan service
	scanConfig := makeScanConfig(*cfg)
	scanServ, err := scanner.NewService(scanConfig, db, log, btcrpc)
	if err != nil {
		log.Println("Open scan service failed:", err)
		return
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		errC <- scanServ.Run()
	}()

	scanCli := scanner.NewScanner(scanServ)

	skycli := cli.New(cfg.Skynode.WalletPath, cfg.Skynode.RPCAddress)

	// create skycoin send service
	sendServ := sender.NewService(makeSendConfig(*cfg), log, skycli)

	wg.Add(1)
	go func() {
		defer wg.Done()
		errC <- sendServ.Run()
	}()

	sendCli := sender.NewSender(sendServ)

	// create exchange service
	exchangeServ := exchange.NewService(makeExchangeConfig(*cfg), db, log, scanCli, sendCli)
	wg.Add(1)
	go func() {
		defer wg.Done()
		errC <- exchangeServ.Run()
	}()

	excli := exchange.NewClient(exchangeServ)

	// create bitcoin address manager
	f, err := ioutil.ReadFile("btc_addresses.json")
	if err != nil {
		log.Println("Load deposit bitcoin address list failed:", err)
		return
	}

	btcAddrGen, err := btcaddrs.New(db, bytes.NewReader(f), log)
	if err != nil {
		log.Println("Create bitcoin deposit address manager failed:", err)
		return
	}

	srv := service.New(makeServiceConfig(*cfg), auth, log, excli, btcAddrGen)

	// Run the service
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.Run()
	}()

	select {
	case <-quit:
	case err := <-errC:
		if err != nil {
			log.Println(err)
		}
	}

	log.Println("Shutting down...")

	// close the scan service
	scanServ.Shutdown()

	// close the skycoin send service
	sendServ.Shutdown()

	// close exchange service
	exchangeServ.Shutdown()

	// close the teller service
	srv.Shutdown()
	wg.Wait()
	log.Println("Shutdown complete")
}

func makeServiceConfig(cfg config.Config) service.Config {
	return service.Config{
		ProxyAddr: cfg.ProxyAddress,

		ReconnectTime: cfg.ReconnectTime,
		PingTimeout:   cfg.PingTimeout,
		PongTimeout:   cfg.PongTimeout,
		DialTimeout:   cfg.DialTimeout,
	}
}

func makeExchangeConfig(cfg config.Config) exchange.Config {
	return exchange.Config{
		Rate: cfg.ExchangeRate,
	}
}

func makeScanConfig(cfg config.Config) scanner.Config {
	return scanner.Config{
		ScanPeriod:        cfg.Btcscan.CheckPeriod,
		DepositBuffersize: cfg.Btcscan.DepositBufferSize,
	}
}

func makeBtcrpcConfg(cfg config.Config) btcrpcclient.ConnConfig {
	certs, err := ioutil.ReadFile(cfg.Btcrpc.Cert)
	if err != nil {
		panic(fmt.Sprintf("btc rpc cert file does not exist in %s", cfg.Btcrpc.Cert))
	}

	return btcrpcclient.ConnConfig{
		Endpoint:     "ws",
		Host:         cfg.Btcrpc.Server,
		User:         cfg.Btcrpc.User,
		Pass:         cfg.Btcrpc.Pass,
		Certificates: certs,
	}
}

func makeSendConfig(cfg config.Config) sender.Config {
	return sender.Config{
		ReqBufSize: cfg.SkySender.ReqBuffSize,
	}
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

func catchInterrupt(quit chan<- struct{}) {
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan
	signal.Stop(sigchan)
	close(quit)
}
