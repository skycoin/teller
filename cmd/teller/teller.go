// Skycoin teller, which provides service of monitoring the bitcoin deposite
// and sending skycoin coins
package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	btcrpcclient "github.com/btcsuite/btcd/rpcclient"
	"github.com/google/gops/agent"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"

	"github.com/skycoin/teller/src/addrs"
	"github.com/skycoin/teller/src/config"
	"github.com/skycoin/teller/src/exchange"
	"github.com/skycoin/teller/src/monitor"
	"github.com/skycoin/teller/src/scanner"
	"github.com/skycoin/teller/src/sender"
	"github.com/skycoin/teller/src/teller"
	"github.com/skycoin/teller/src/util/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func createBtcScanner(log logrus.FieldLogger, cfg config.Config, scanStore *scanner.Store) (*scanner.BTCScanner, error) {
	// create btc rpc client
	certs, err := ioutil.ReadFile(cfg.BtcRPC.Cert)
	if err != nil {
		return nil, fmt.Errorf("Failed to read cfg.BtcRPC.Cert %s: %v", cfg.BtcRPC.Cert, err)
	}

	log.Info("Connecting to btcd")

	btcrpc, err := btcrpcclient.New(&btcrpcclient.ConnConfig{
		Endpoint:     "ws",
		Host:         cfg.BtcRPC.Server,
		User:         cfg.BtcRPC.User,
		Pass:         cfg.BtcRPC.Pass,
		Certificates: certs,
	}, nil)
	if err != nil {
		log.WithError(err).Error("Connect btcd failed")
		return nil, err
	}

	log.Info("Connect to btcd succeeded")

	err = scanStore.AddSupportedCoin(scanner.CoinTypeBTC)
	if err != nil {
		log.WithError(err).Error("scanStore.AddSupportedCoin(scanner.CoinTypeBTC) failed")
		return nil, err
	}

	btcScanner, err := scanner.NewBTCScanner(log, scanStore, btcrpc, scanner.Config{
		ScanPeriod:            cfg.BtcScanner.ScanPeriod,
		ConfirmationsRequired: cfg.BtcScanner.ConfirmationsRequired,
		InitialScanHeight:     cfg.BtcScanner.InitialScanHeight,
	})
	if err != nil {
		log.WithError(err).Error("Open scan service failed")
		return nil, err
	}
	return btcScanner, nil
}

func createEthScanner(log logrus.FieldLogger, cfg config.Config, scanStore *scanner.Store) (*scanner.ETHScanner, error) {
	ethrpc, err := scanner.NewEthClient(cfg.EthRPC.Server, cfg.EthRPC.Port)
	if err != nil {
		log.WithError(err).Error("Connect geth failed")
		return nil, err
	}

	err = scanStore.AddSupportedCoin(scanner.CoinTypeETH)
	if err != nil {
		log.WithError(err).Error("scanStore.AddSupportedCoin(scanner.CoinTypeETH) failed")
		return nil, err
	}

	ethScanner, err := scanner.NewETHScanner(log, scanStore, ethrpc, scanner.Config{
		ScanPeriod:            cfg.EthScanner.ScanPeriod,
		ConfirmationsRequired: cfg.EthScanner.ConfirmationsRequired,
		InitialScanHeight:     cfg.EthScanner.InitialScanHeight,
	})
	if err != nil {
		log.WithError(err).Error("Open ethscan service failed")
		return nil, err
	}
	return ethScanner, nil
}

func run() error {
	cur, err := user.Current()
	if err != nil {
		fmt.Println("Failed to get user's home directory:", err)
		return err
	}
	defaultAppDir := filepath.Join(cur.HomeDir, ".teller-skycoin")

	appDirOpt := pflag.StringP("dir", "d", defaultAppDir, "application data directory")
	configNameOpt := pflag.StringP("config", "c", "config", "name of configuration file")
	pflag.Parse()

	if err := createFolderIfNotExist(*appDirOpt); err != nil {
		fmt.Println("Create application data directory failed:", err)
		return err
	}

	cfg, err := config.Load(*configNameOpt, *appDirOpt)
	if err != nil {
		return fmt.Errorf("Config error:\n%v", err)
	}

	// Init logger
	rusloggger, err := logger.NewLogger(cfg.LogFilename, cfg.Debug)
	if err != nil {
		fmt.Println("Failed to create Logrus logger:", err)
		return err
	}

	log := rusloggger.WithField("prefix", "teller")

	log.WithField("config", cfg.Redacted()).Info("Loaded teller config")

	if cfg.Profile {
		// Start gops agent, for profiling
		if err := agent.Listen(&agent.Options{
			NoShutdownCleanup: true,
		}); err != nil {
			log.WithError(err).Error("Start profile agent failed")
			return err
		}
	}

	quit := make(chan struct{})
	go catchInterrupt(quit)

	// Open db
	dbPath := filepath.Join(*appDirOpt, cfg.DBFilename)
	db, err := bolt.Open(dbPath, 0700, &bolt.Options{
		Timeout: 1 * time.Second,
	})
	if err != nil {
		log.WithError(err).Error("Open db failed")
		return err
	}

	errC := make(chan error, 20)
	var wg sync.WaitGroup

	background := func(name string, errC chan<- error, f func() error) {
		log.Infof("Backgrounding task %s", name)
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := f()
			if err != nil {
				log.WithError(err).Errorf("Backgrounded task %s failed", name)
				errC <- fmt.Errorf("Backgrounded task %s failed: %v", name, err)
			} else {
				log.Infof("Backgrounded task %s shutdown", name)
			}
		}()
	}

	var btcScanner *scanner.BTCScanner
	var ethScanner *scanner.ETHScanner
	var scanService scanner.Scanner
	var scanEthService scanner.Scanner
	var sendService *sender.SendService
	var sendRPC sender.Sender
	var btcAddrMgr *addrs.Addrs
	var ethAddrMgr *addrs.Addrs

	//create multiplexer to manage scanner
	multiplexer := scanner.NewMultiplexer(log)

	dummyMux := http.NewServeMux()

	// create scan storer
	scanStore, err := scanner.NewStore(log, db)
	if err != nil {
		log.WithError(err).Error("scanner.NewStore failed")
		return err
	}

	if cfg.Dummy.Scanner {
		log.Info("btcd disabled, running dummy scanner")
		scanService = scanner.NewDummyScanner(log)
		scanService.(*scanner.DummyScanner).RegisterCoinType(scanner.CoinTypeBTC)
		// TODO -- refactor dummy scanning to support multiple coin types
		// scanEthService = scanner.NewDummyScanner(log)
		scanService.(*scanner.DummyScanner).BindHandlers(dummyMux)
	} else {
		// enable btc scanner
		if cfg.BtcRPC.Enabled {
			btcScanner, err = createBtcScanner(rusloggger, cfg, scanStore)
			if err != nil {
				log.WithError(err).Error("create btc scanner failed")
				return err
			}
			background("btcScanner.Run", errC, btcScanner.Run)

			scanService = btcScanner
		}

		// enable eth scanner
		if cfg.EthRPC.Enabled {
			ethScanner, err = createEthScanner(rusloggger, cfg, scanStore)
			if err != nil {
				log.WithError(err).Error("create eth scanner failed")
				return err
			}

			background("ethScanner.Run", errC, ethScanner.Run)

			scanEthService = ethScanner

			if err := multiplexer.AddScanner(scanEthService, scanner.CoinTypeETH); err != nil {
				log.WithError(err).Errorf("multiplexer.AddScanner of %s failed", scanner.CoinTypeETH)
				return err
			}
		}
	}

	if err := multiplexer.AddScanner(scanService, scanner.CoinTypeBTC); err != nil {
		log.WithError(err).Errorf("multiplexer.AddScanner of %s failed", scanner.CoinTypeBTC)
		return err
	}

	background("multiplex.Run", errC, multiplexer.Multiplex)

	if cfg.Dummy.Sender {
		log.Info("skyd disabled, running dummy sender")
		sendRPC = sender.NewDummySender(log)
		sendRPC.(*sender.DummySender).BindHandlers(dummyMux)
	} else {
		skyClient, err := sender.NewRPC(cfg.SkyExchanger.Wallet, cfg.SkyRPC.Address)
		if err != nil {
			log.WithError(err).Error("sender.NewRPC failed")
			return err
		}

		sendService = sender.NewService(log, skyClient)

		background("sendService.Run", errC, sendService.Run)

		sendRPC = sender.NewRetrySender(sendService)
	}

	if cfg.Dummy.Scanner || cfg.Dummy.Sender {
		log.Infof("Starting dummy admin interface listener on http://%s", cfg.Dummy.HTTPAddr)
		go func() {
			if err := http.ListenAndServe(cfg.Dummy.HTTPAddr, dummyMux); err != nil {
				log.WithError(err).Error("Dummy ListenAndServe failed")
			}
		}()
	}

	// create exchange service
	exchangeStore, err := exchange.NewStore(log, db)
	if err != nil {
		log.WithError(err).Error("exchange.NewStore failed")
		return err
	}

	var exchangeClient *exchange.Exchange

	switch cfg.SkyExchanger.BuyMethod {
	case config.BuyMethodDirect:
		var err error
		exchangeClient, err = exchange.NewDirectExchange(log, cfg.SkyExchanger, exchangeStore, multiplexer, sendRPC)
		if err != nil {
			log.WithError(err).Error("exchange.NewDirectExchange failed")
			return err
		}
	case config.BuyMethodPassthrough:
		var err error
		exchangeClient, err = exchange.NewPassthroughExchange(log, cfg.SkyExchanger, exchangeStore, multiplexer, sendRPC)
		if err != nil {
			log.WithError(err).Error("exchange.NewPassthroughExchange failed")
			return err
		}
	default:
		log.WithError(config.ErrInvalidBuyMethod).Error()
		return config.ErrInvalidBuyMethod
	}

	background("exchangeClient.Run", errC, exchangeClient.Run)

	// create AddrManager
	addrManager := addrs.NewAddrManager()

	if cfg.BtcRPC.Enabled {
		// create bitcoin address manager
		f, err := ioutil.ReadFile(cfg.BtcAddresses)
		if err != nil {
			log.WithError(err).Error("Load deposit bitcoin address list failed")
			return err
		}

		btcAddrMgr, err = addrs.NewBTCAddrs(log, db, bytes.NewReader(f))
		if err != nil {
			log.WithError(err).Error("Create bitcoin deposit address manager failed")
			return err
		}
		if err := addrManager.PushGenerator(btcAddrMgr, scanner.CoinTypeBTC); err != nil {
			log.WithError(err).Error("add btc address manager failed")
			return err
		}
	}

	if cfg.EthRPC.Enabled {
		// create ethcoin address manager
		f, err := ioutil.ReadFile(cfg.EthAddresses)
		if err != nil {
			log.WithError(err).Error("Load deposit ethcoin address list failed")
			return err
		}

		ethAddrMgr, err = addrs.NewETHAddrs(log, db, bytes.NewReader(f))
		if err != nil {
			log.WithError(err).Error("Create ethcoin deposit address manager failed")
			return err
		}
		if err := addrManager.PushGenerator(ethAddrMgr, scanner.CoinTypeETH); err != nil {
			log.WithError(err).Error("add eth address manager failed")
			return err
		}
	}

	tellerServer := teller.New(log, exchangeClient, addrManager, cfg)

	// Run the service
	background("tellerServer.Run", errC, tellerServer.Run)

	// start monitor service
	monitorCfg := monitor.Config{
		Addr: cfg.AdminPanel.Host,
	}
	monitorService := monitor.New(log, monitorCfg, btcAddrMgr, ethAddrMgr, exchangeClient, btcScanner)

	background("monitorService.Run", errC, monitorService.Run)

	var finalErr error
	select {
	case <-quit:
	case finalErr = <-errC:
		if finalErr != nil {
			log.WithError(finalErr).Error("Goroutine error")
		}
	}

	log.Info("Shutting down...")

	if monitorService != nil {
		log.Info("Shutting down monitorService")
		monitorService.Shutdown()
	}

	// close the teller service
	log.Info("Shutting down tellerServer")
	tellerServer.Shutdown()

	log.Info("Shutting down the multiplexer")
	multiplexer.Shutdown()

	// close the scan service
	if btcScanner != nil {
		log.Info("Shutting down btcScanner")
		btcScanner.Shutdown()
	}
	// close the scan service
	if ethScanner != nil {
		log.Info("Shutting down ethScanner")
		ethScanner.Shutdown()
	}

	// close exchange service
	log.Info("Shutting down exchangeClient")
	exchangeClient.Shutdown()

	// close the skycoin send service
	if sendService != nil {
		log.Info("Shutting down sendService")
		sendService.Shutdown()
	}

	log.Info("Waiting for goroutines to exit")

	wg.Wait()

	log.Info("Shutdown complete")

	return finalErr
}

func createFolderIfNotExist(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// create the dir
		if err := os.Mkdir(path, 0700); err != nil {
			return err
		}
	}
	return nil
}

func printProgramStatus() {
	p := pprof.Lookup("goroutine")
	if err := p.WriteTo(os.Stdout, 2); err != nil {
		fmt.Println("ERROR:", err)
		return
	}
}

func catchInterrupt(quit chan<- struct{}) {
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan
	signal.Stop(sigchan)
	close(quit)

	// If ctrl-c is called again, panic so that the program state can be examined.
	// Ctrl-c would be called again if program shutdown was stuck.
	go catchInterruptPanic()
}

// catchInterruptPanic catches os.Interrupt and panics
func catchInterruptPanic() {
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan
	signal.Stop(sigchan)
	printProgramStatus()
	panic("SIGINT")
}
