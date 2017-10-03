// Skycoin teller, which provides service of monitoring the bitcoin deposite
// and sending skycoin coins
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/btcsuite/btcrpcclient"
	"github.com/google/gops/agent"
	"github.com/sirupsen/logrus"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
	"github.com/skycoin/teller/src/service"
	"github.com/skycoin/teller/src/service/btcaddrs"
	"github.com/skycoin/teller/src/service/config"
	"github.com/skycoin/teller/src/service/exchange"
	"github.com/skycoin/teller/src/service/monitor"
	"github.com/skycoin/teller/src/service/rpc"
	"github.com/skycoin/teller/src/service/scanner"
	"github.com/skycoin/teller/src/service/sender"
)

const (
	appDir = ".skycoin-teller"
	dbName = "data.db"
)

type dummyBtcScanner struct {
	log *logrus.Logger
}

func (s *dummyBtcScanner) AddScanAddress(addr string) error {
	s.log.WithField("addr", addr).Info("dummyBtcScanner.AddDepositAddress")
	return nil
}

func (s *dummyBtcScanner) GetDepositValue() <-chan scanner.DepositNote {
	s.log.Info("dummyBtcScanner.GetDepositValue")
	c := make(chan scanner.DepositNote)
	close(c)
	return c
}

func (s *dummyBtcScanner) GetScanAddresses() ([]string, error) {
	return []string{}, nil
}

type dummySkySender struct {
	log *logrus.Logger
}

func (s *dummySkySender) Send(destAddr string, coins uint64, opt *sender.SendOption) (string, error) {
	s.log.WithFields(logrus.Fields{
		"destAddr": destAddr,
		"coins":    coins,
		"opt":      opt,
	}).Info("dummySkySender.Send")
	return "", errors.New("dummy sky sender")
}

func (s *dummySkySender) SendAsync(destAddr string, coins uint64, opt *sender.SendOption) (<-chan interface{}, error) {
	s.log.WithFields(logrus.Fields{
		"destAddr": destAddr,
		"coins":    coins,
		"opt":      opt,
	}).Info("dummySkySender.SendAsync")
	return nil, errors.New("dummy sky sender")
}

func (s *dummySkySender) IsClosed() bool {
	return true
}

func (s *dummySkySender) IsTxConfirmed(txid string) bool {
	return true
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	configFile := flag.String("cfg", "config.json", "config.json file")
	btcAddrs := flag.String("btc-addrs", "btc_addresses.json", "btc_addresses.json file")
	proxyPubkey := flag.String("proxy-pubkey", "", "proxy pubkey")
	debug := flag.Bool("debug", false, "debug mode will show more detail logs")
	dummyMode := flag.Bool("dummy", false, "run without real btcd or skyd service")
	profile := flag.Bool("prof", false, "start gops profiling tool")
	logFilename := flag.String("log-file", "teller.log", "teller log filename")

	flag.Parse()

	// init logger
	log, err := logger.NewLogger(*logFilename, *debug)
	if err != nil {
		fmt.Println("Failed to create Logrus logger:", err)
		return err
	}

	if *proxyPubkey == "" {
		log.Error("-proxy-pubkey missing")
		return errors.New("-proxy-pubkey missing")
	}

	rpubkey, err := cipher.PubKeyFromHex(*proxyPubkey)
	if err != nil {
		log.WithError(err).Error("Invalid proxy pubkey")
		return err
	}

	// generate local private key
	_, lseckey := cipher.GenerateKeyPair()

	auth := &daemon.Auth{
		RPubkey: rpubkey,
		LSeckey: lseckey,
	}

	if *profile {
		// start gops agent, for profilling
		if err := agent.Listen(&agent.Options{
			NoShutdownCleanup: true,
		}); err != nil {
			log.WithError(err).Error("Start profile agent failed")
			return err
		}
	}

	quit := make(chan struct{})
	go catchInterrupt(quit)

	// load config
	cfg, err := config.New(*configFile)
	if err != nil {
		log.WithError(err).Error("Load config failed")
		return err
	}

	appDir, err := createAppDirIfNotExist(appDir)
	if err != nil {
		log.WithError(err).Error("Create AppDir failed")
		return err
	}

	// open db
	dbPath := filepath.Join(appDir, dbName)
	db, err := bolt.Open(dbPath, 0700, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.WithError(err).Error("Open db failed")
		return err
	}

	errC := make(chan error, 10)
	wg := sync.WaitGroup{}

	var scanServ *scanner.ScanService
	var scanRPC exchange.BtcScanner
	var sendServ *sender.SendService
	var sendRPC exchange.SkySender

	if *dummyMode {
		log.Info("btcd and skyd disabled, running in dummy mode")
		scanRPC = &dummyBtcScanner{log: log}
		sendRPC = &dummySkySender{log: log}
	} else {
		// check skycoin setup
		if err := checkSkycoinSetup(*cfg); err != nil {
			log.Println(err)
			return err
		}

		// create btc rpc client
		btcrpcConnConf := makeBtcrpcConfg(*cfg)
		btcrpc, err := btcrpcclient.New(&btcrpcConnConf, nil)
		if err != nil {
			log.WithError(err).Error("Connect btcd failed")
			return err
		}

		log.Info("Connect to btcd success")

		// create scan service
		scanConfig := makeScanConfig(*cfg)
		scanServ, err = scanner.NewService(scanConfig, db, log, btcrpc)
		if err != nil {
			log.WithError(err).Error("Open scan service failed")
			return err
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			errC <- scanServ.Run()
		}()

		scanRPC = scanner.NewScanner(scanServ)

		skyRPC := rpc.New(cfg.Skynode.WalletPath, cfg.Skynode.RPCAddress)

		// create skycoin send service
		sendServ = sender.NewService(makeSendConfig(*cfg), log, skyRPC)

		wg.Add(1)
		go func() {
			defer wg.Done()
			errC <- sendServ.Run()
		}()

		sendRPC = sender.NewSender(sendServ)
	}

	// create exchange service
	exchangeServ := exchange.NewService(makeExchangeConfig(*cfg), db, log, scanRPC, sendRPC)
	wg.Add(1)
	go func() {
		defer wg.Done()
		errC <- exchangeServ.Run()
	}()

	excCli := exchange.NewClient(exchangeServ)

	// create bitcoin address manager
	f, err := ioutil.ReadFile(*btcAddrs)
	if err != nil {
		log.WithError(err).Error("Load deposit bitcoin address list failed")
		return err
	}

	btcAddrMgr, err := btcaddrs.New(db, bytes.NewReader(f), log)
	if err != nil {
		log.WithError(err).Error("Create bitcoin deposit address manager failed")
		return err
	}

	srv := service.New(makeServiceConfig(*cfg), auth, log, excCli, btcAddrMgr)

	// Run the service
	wg.Add(1)
	go func() {
		defer wg.Done()
		errC <- srv.Run()
	}()

	// start monitor service
	monitorCfg := monitor.Config{
		Addr: cfg.MonitorAddr,
	}
	ms := monitor.New(monitorCfg, log, btcAddrMgr, excCli, scanRPC)

	wg.Add(1)
	go func() {
		defer wg.Done()
		errC <- ms.Run()
	}()

	var finalErr error
	select {
	case <-quit:
	case finalErr = <-errC:
		if finalErr != nil {
			log.WithError(finalErr).Error()
		}
	}

	log.Info("Shutting down...")

	if ms != nil {
		ms.Shutdown()
	}

	// close the skycoin send service
	if sendServ != nil {
		sendServ.Shutdown()
	}

	// close exchange service
	exchangeServ.Shutdown()

	// close the teller service
	srv.Shutdown()

	// close the scan service
	if scanServ != nil {
		scanServ.Shutdown()
	}

	wg.Wait()
	log.Info("Shutdown complete")

	return finalErr
}

func makeServiceConfig(cfg config.Config) service.Config {
	return service.Config{
		ProxyAddr: cfg.ProxyAddress,

		ReconnectTime: cfg.ReconnectTime,
		PingTimeout:   cfg.PingTimeout,
		PongTimeout:   cfg.PongTimeout,
		DialTimeout:   cfg.DialTimeout,

		MaxBind:             cfg.MaxBind,
		SessionWriteBufSize: cfg.SessionWriteBufSize,
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

// checks skycoin setups
func checkSkycoinSetup(cfg config.Config) error {
	// check whether the skycoin wallet file does exist
	if _, err := os.Stat(cfg.Skynode.WalletPath); os.IsNotExist(err) {
		return fmt.Errorf("skycoin wallet file: %s does not exist", cfg.Skynode.WalletPath)
	}

	// test if skycoin node rpc service is reachable
	conn, err := net.Dial("tcp", cfg.Skynode.RPCAddress)
	if err != nil {
		return fmt.Errorf("connect to skycoin node %s failed: %v", cfg.Skynode.RPCAddress, err)
	}

	conn.Close()

	return nil
}
