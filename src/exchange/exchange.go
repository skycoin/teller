package exchange

import (
	"errors"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/MDLlife/MDL/src/api/cli"

	"github.com/MDLlife/teller/src/config"
	"github.com/MDLlife/teller/src/scanner"
	"github.com/MDLlife/teller/src/sender"
)

const (
	txConfirmationCheckWait = time.Second * 3
)

var (
	// ErrEmptySendAmount is returned if the calculated mdl amount to send is 0
	ErrEmptySendAmount = errors.New("MDL send amount is 0")
	// ErrNoResponse is returned when the send service returns a nil response. This happens if the send service has closed.
	ErrNoResponse = errors.New("No response from the send service")
	// ErrNotConfirmed is returned if the tx is not confirmed yet
	ErrNotConfirmed = errors.New("Transaction is not confirmed yet")
	// ErrDepositStatusInvalid is returned when handling a deposit with a status that cannot be processed
	// This includes StatusWaitDeposit and StatusUnknown
	ErrDepositStatusInvalid = errors.New("Deposit status cannot be handled")
	// ErrNoBoundAddress is returned if no mdl address is bound to a deposit's address
	ErrNoBoundAddress = errors.New("Deposit has no bound mdl address")
	// ErrLowExchangeBalance is returned if the trading exchange is supposed to have more coins than it does.
	ErrLowExchangeBalance = errors.New("Exchange has less coins than it should")
	// ErrNoAsksAvailable is returned if there are no ask orders available on the exchange orderbook
	ErrNoAsksAvailable = errors.New("No ask orders available")
)

// DepositFilter filters deposits
type DepositFilter func(di DepositInfo) bool

// Runner defines an interface for components that can be started and stopped
type Runner interface {
	Run() error
	Shutdown()
}

// Exchanger provides APIs to interact with the exchange service
type Exchanger interface {
	BindAddress(mdlAddr, depositAddr, coinType string) (*BoundAddress, error)
	GetDepositStatuses(mdlAddr string) ([]DepositStatus, error)
	GetDepositStatusDetail(flt DepositFilter) ([]DepositStatusDetail, error)
	GetBindNum(mdlAddr string) (int, error)
	GetDepositStats() (*DepositStats, error)
	Status() error
	Balance() (*cli.Balance, error)
}

// Exchange encompasses an entire coin<>mdl deposit-process-send flow
type Exchange struct {
	log   logrus.FieldLogger
	store Storer
	cfg   config.MDLExchanger
	quit  chan struct{}
	done  chan struct{}

	Receiver  ReceiveRunner
	Processor ProcessRunner
	Sender    SendRunner
}

// NewDirectExchange creates an Exchange which performs "direct buy", i.e. directly selling from a local mdl wallet
func NewDirectExchange(log logrus.FieldLogger, cfg config.MDLExchanger, store Storer, multiplexer *scanner.Multiplexer, coinSender sender.Sender) (*Exchange, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if cfg.BuyMethod != config.BuyMethodDirect {
		return nil, config.ErrInvalidBuyMethod
	}

	receiver, err := NewReceive(log, cfg, store, multiplexer)
	if err != nil {
		return nil, err
	}

	processor, err := NewDirectBuy(log, cfg, store, receiver)
	if err != nil {
		return nil, err
	}

	sender, err := NewSend(log, cfg, store, coinSender, processor)
	if err != nil {
		return nil, err
	}

	return &Exchange{
		log:       log.WithField("prefix", "teller.exchange.exchange"),
		store:     store,
		cfg:       cfg,
		quit:      make(chan struct{}),
		done:      make(chan struct{}),
		Receiver:  receiver,
		Processor: processor,
		Sender:    sender,
	}, nil
}

// NewPassthroughExchange creates an Exchange which performs "passthrough buy",
// i.e. it purchases coins from an exchange before sending from a local mdl wallet
func NewPassthroughExchange(log logrus.FieldLogger, cfg config.MDLExchanger, store Storer, multiplexer *scanner.Multiplexer, coinSender sender.Sender) (*Exchange, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if cfg.BuyMethod != config.BuyMethodPassthrough {
		return nil, config.ErrInvalidBuyMethod
	}

	receiver, err := NewReceive(log, cfg, store, multiplexer)
	if err != nil {
		return nil, err
	}

	processor, err := NewPassthrough(log, cfg, store, receiver)
	if err != nil {
		return nil, err
	}

	sender, err := NewSend(log, cfg, store, coinSender, processor)
	if err != nil {
		return nil, err
	}

	return &Exchange{
		log:       log.WithField("prefix", "teller.exchange.exchange"),
		store:     store,
		cfg:       cfg,
		quit:      make(chan struct{}),
		done:      make(chan struct{}),
		Receiver:  receiver,
		Processor: processor,
		Sender:    sender,
	}, nil
}

// Run runs all components of the Exchange
func (e *Exchange) Run() error {
	e.log.Info("Start exchange service...")
	defer func() {
		e.log.Info("Closed exchange service")
		e.done <- struct{}{}
	}()

	// TODO: Alternative way of managing the subcomponents:
	// Create channels for linking two components, initialize the components with the channels
	// Close them to teardown

	errC := make(chan error, 3)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := e.Receiver.Run(); err != nil {
			e.log.WithError(err).Error("Receiver.Run failed")
			errC <- err
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := e.Processor.Run(); err != nil {
			e.log.WithError(err).Error("Processor.Run failed")
			errC <- err
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := e.Sender.Run(); err != nil {
			e.log.WithError(err).Error("Sender.Run failed")
			errC <- err
		}
	}()

	var err error
	select {
	case <-e.quit:
	case err = <-errC:
		e.log.WithError(err).Error("Terminating early")
	}

	wg.Wait()

	return err
}

// Shutdown stops a previous call to run
func (e *Exchange) Shutdown() {
	e.log.Info("Shutting down Exchange")
	close(e.quit)

	e.log.Info("Shutting down Exchange subcomponents")
	e.Receiver.Shutdown()
	e.Processor.Shutdown()
	e.Sender.Shutdown()

	e.log.Info("Waiting for run to finish")
	<-e.done
	e.log.Info("Shutdown complete")
}

// DepositStatus json struct for deposit status
type DepositStatus struct {
	Seq       uint64 `json:"seq"`
	UpdatedAt int64  `json:"updated_at"`
	Status    string `json:"status"`
	CoinType  string `json:"coin_type"`
}

// DepositStatusDetail deposit status detail info
type DepositStatusDetail struct {
	Seq            uint64 `json:"seq"`
	UpdatedAt      int64  `json:"updated_at"`
	Status         string `json:"status"`
	MDLAddress     string `json:"mdl_address"`
	DepositAddress string `json:"deposit_address"`
	CoinType       string `json:"coin_type"`
	Txid           string `json:"txid"`
}

// GetDepositStatuses returns deamon.DepositStatus array of given mdl address
func (e *Exchange) GetDepositStatuses(mdlAddr string) ([]DepositStatus, error) {
	dis, err := e.store.GetDepositInfoOfMDLAddress(mdlAddr)
	if err != nil {
		return []DepositStatus{}, err
	}

	dss := make([]DepositStatus, 0, len(dis))
	for _, di := range dis {
		dss = append(dss, DepositStatus{
			Seq:       di.Seq,
			UpdatedAt: di.UpdatedAt,
			Status:    di.Status.String(),
			CoinType:  di.CoinType,
		})
	}
	return dss, nil
}

// GetDepositStatusDetail returns deposit status details
func (e *Exchange) GetDepositStatusDetail(flt DepositFilter) ([]DepositStatusDetail, error) {
	dis, err := e.store.GetDepositInfoArray(flt)
	if err != nil {
		return nil, err
	}

	dss := make([]DepositStatusDetail, 0, len(dis))
	for _, di := range dis {
		dss = append(dss, DepositStatusDetail{
			Seq:            di.Seq,
			UpdatedAt:      di.UpdatedAt,
			Status:         di.Status.String(),
			MDLAddress:     di.MDLAddress,
			DepositAddress: di.DepositAddress,
			Txid:           di.Txid,
			CoinType:       di.CoinType,
		})
	}
	return dss, nil
}

// GetBindNum returns the number of btc/eth address the given mdl address binded
func (e *Exchange) GetBindNum(mdlAddr string) (int, error) {
	addrs, err := e.store.GetMDLBindAddresses(mdlAddr)
	return len(addrs), err
}

// GetDepositStats returns deposit status
func (e *Exchange) GetDepositStats() (*DepositStats, error) {
	tbr, tss, err := e.store.GetDepositStats()
	if err != nil {
		return nil, err
	}

	return &DepositStats{
		TotalBTCReceived: tbr,
		TotalMDLSent:     tss,
	}, nil
}

// Balance returns the number of coins left in the OTC wallet
func (e *Exchange) Balance() (*cli.Balance, error) {
	return e.Sender.Balance()
}

// Status returns the last return value of the processing state
func (e *Exchange) Status() error {
	return e.Sender.Status()
}

// BindAddress binds deposit address with mdl address, and
// add the btc/eth address to scan service, when detect deposit coin
// to the btc/eth address, will send specific mdl to the binded
// mdl address
func (e *Exchange) BindAddress(mdlAddr, depositAddr, coinType string) (*BoundAddress, error) {
	return e.Receiver.BindAddress(mdlAddr, depositAddr, coinType, e.cfg.BuyMethod)
}
