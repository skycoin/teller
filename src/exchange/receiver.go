package exchange

import (
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/skycoin/teller/src/config"
	"github.com/skycoin/teller/src/scanner"
)

func init() {
	// Assert that getRate() handles all coin types
	cfg := config.SkyExchanger{
		SkyBtcExchangeRate: "1",
		SkyEthExchangeRate: "2",
	}
	for _, ct := range scanner.GetCoinTypes() {
		rate, err := getRate(cfg, ct)
		if err != nil {
			panic(err)
		}
		if rate == "" {
			panic(fmt.Sprintf("getRate(%s) did not find a rate", ct))
		}
	}
}

// Receiver is a component that reads deposits from a scanner.Scanner and records them
type Receiver interface {
	Deposits() <-chan DepositInfo
	BindAddress(skyAddr, depositAddr, coinType, buyMethod string) (*BoundAddress, error)
}

// ReceiveRunner is a Receiver than can be run
type ReceiveRunner interface {
	Runner
	Receiver
}

// Receive implements a Receiver. All incoming deposits are saved,
// with the configured rate recorded at instantiation time [TODO: move that functionality to Processor?]
type Receive struct {
	log         logrus.FieldLogger
	cfg         config.SkyExchanger
	multiplexer *scanner.Multiplexer
	store       Storer
	deposits    chan DepositInfo
	quit        chan struct{}
	done        chan struct{}
}

// NewReceive creates a Receive
func NewReceive(log logrus.FieldLogger, cfg config.SkyExchanger, store Storer, multiplexer *scanner.Multiplexer) (*Receive, error) {
	// TODO -- split up config into relevant parts?
	// The Receive component needs exchange rates
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Receive{
		log:         log.WithField("prefix", "teller.exchange.Receive"),
		cfg:         cfg,
		store:       store,
		multiplexer: multiplexer,
		deposits:    make(chan DepositInfo, 100),
		quit:        make(chan struct{}),
		done:        make(chan struct{}),
	}, nil
}

// Run processes deposits from the scanner.Scanner, recording them and exposing them over the Deposits() channel
func (r *Receive) Run() error {
	log := r.log
	log.Info("Start receive service...")
	defer func() {
		log.Info("Closed receive service")
		r.done <- struct{}{}
	}()

	// Load StatusWaitDecide deposits for resubmission
	waitDecideDeposits, err := r.store.GetDepositInfoArray(func(di DepositInfo) bool {
		return di.Status == StatusWaitDecide
	})

	if err != nil {
		err = fmt.Errorf("GetDepositInfoArray failed: %v", err)
		log.WithError(err).Error(err)
		return err
	}

	// Queue the saved StatusWaitDecide deposits
	// This will block if there are too many waiting deposits, make sure that
	// the Processor is running to receive them
	for _, di := range waitDecideDeposits {
		r.deposits <- di
	}

	var wg sync.WaitGroup

	// This loop processes incoming deposits from the scanner and saves a
	// new DepositInfo with a status of StatusWaitSend
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.runReadMultiplexer()
	}()

	wg.Wait()

	return nil
}

// runReadMultiplexer reads deposits from the multiplexer
func (r *Receive) runReadMultiplexer() {
	log := r.log.WithField("goroutine", "readMultiplexer")
	for {
		var dv scanner.DepositNote
		var ok bool
		select {
		case <-r.quit:
			log.Info("quit")
			return
		case dv, ok = <-r.multiplexer.GetDeposit():
			if !ok {
				log.Warn("Scan service closed, watch deposits loop quit")
				return
			}

		}
		log := log.WithField("deposit", dv.Deposit)

		// Save a new DepositInfo based upon the scanner.Deposit.
		// If the save fails, report it to the scanner.
		// The scanner will mark the deposit as "processed" if no error
		// occurred.  Any unprocessed deposits held by the scanner
		// will be resent to the exchange when teller is started.
		if d, err := r.saveIncomingDeposit(dv.Deposit); err != nil {
			log.WithError(err).Error("saveIncomingDeposit failed. This deposit will not be reprocessed until teller is restarted.")
			dv.ErrC <- err
		} else {
			dv.ErrC <- nil
			r.deposits <- d
		}
	}
}

// Shutdown stops a previous call to run
func (r *Receive) Shutdown() {
	r.log.Info("Shutting down Receive")
	close(r.quit)
	r.log.Info("Waiting for run to finish")
	<-r.done
	r.log.Info("Shutdown complete")
}

// Deposits returns a channel with recorded deposits
func (r *Receive) Deposits() <-chan DepositInfo {
	return r.deposits
}

// saveIncomingDeposit is called when receiving a deposit from the scanner
func (r *Receive) saveIncomingDeposit(dv scanner.Deposit) (DepositInfo, error) {
	log := r.log.WithField("deposit", dv)

	var rate string
	rate, err := r.getRate(dv.CoinType)
	if err != nil {
		log.WithError(err).Error("get conversion rate failed")
		return DepositInfo{}, err
	}

	di, err := r.store.GetOrCreateDepositInfo(dv, rate)
	if err != nil {
		log.WithError(err).Error("GetOrCreateDepositInfo failed")
		return DepositInfo{}, err
	}

	log = log.WithField("depositInfo", di)
	log.Info("Saved DepositInfo")

	return di, err
}

// getRate returns conversion rate according to coin type
func (r *Receive) getRate(coinType string) (string, error) {
	return getRate(r.cfg, coinType)
}

// getRate returns conversion rate according to coin type
func getRate(cfg config.SkyExchanger, coinType string) (string, error) {
	switch coinType {
	case scanner.CoinTypeBTC:
		return cfg.SkyBtcExchangeRate, nil
	case scanner.CoinTypeETH:
		return cfg.SkyEthExchangeRate, nil
	default:
		return "", scanner.ErrUnsupportedCoinType
	}
}

// BindAddress binds deposit address with skycoin address, and
// add the btc/eth address to scan service, when detect deposit coin
// to the btc/eth address, will send specific skycoin to the binded
// skycoin address
func (r *Receive) BindAddress(skyAddr, depositAddr, coinType, buyMethod string) (*BoundAddress, error) {
	if err := config.ValidateBuyMethod(buyMethod); err != nil {
		return nil, err
	}

	if err := r.multiplexer.ValidateCoinType(coinType); err != nil {
		return nil, err
	}

	boundAddr, err := r.store.BindAddress(skyAddr, depositAddr, coinType, buyMethod)
	if err != nil {
		return nil, err
	}

	if err := r.multiplexer.AddScanAddress(depositAddr, coinType); err != nil {
		return nil, err
	}

	return boundAddr, nil
}
