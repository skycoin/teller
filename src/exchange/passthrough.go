package exchange

import (
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/skycoin/teller/src/config"
)

// Passthrough implements a Processor. For each deposit, it buys a corresponding amount
// from c2cx.com, then tells the sender to send the amount bought.
type Passthrough struct {
	log        logrus.FieldLogger
	cfg        config.SkyExchanger
	receiver   Receiver
	store      Storer
	deposits   chan DepositInfo
	quit       chan struct{}
	done       chan struct{}
	statusLock sync.RWMutex
	status     error
}

// NewPassthrough creates Passthrough
func NewPassthrough(log logrus.FieldLogger, cfg config.SkyExchanger, store Storer, receiver Receiver) (*Passthrough, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Passthrough{
		log:      log.WithField("prefix", "teller.exchange.passthrough"),
		cfg:      cfg,
		store:    store,
		receiver: receiver,
		deposits: make(chan DepositInfo, 100),
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
	}, nil
}

// Run begins the Passthrough service
func (p *Passthrough) Run() error {
	log := p.log
	log.Info("Start passthrough service...")
	defer func() {
		log.Info("Closed passthrough service")
		p.done <- struct{}{}
	}()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		p.runBuy()
	}()

	wg.Wait()

	return nil
}

func (p *Passthrough) runBuy() {
	log := p.log.WithField("goroutine", "runBuy")
	for {
		select {
		case <-p.quit:
			log.Info("quit")
			return
		case d := <-p.receiver.Deposits():
			// TODO -- buy from the exchange
			updatedDeposit, err := p.processWaitDecideDeposit(d)
			if err != nil {
				msg := "handleDeposit failed. This deposit wil not be reprocessed until teller is restarted."
				log.WithField("depositInfo", d).WithError(err).Error(msg)
				continue
			}

			p.deposits <- updatedDeposit
		}
	}
}

// Shutdown stops a previous call to Run
func (p *Passthrough) Shutdown() {
	p.log.Info("Shutting down Passthrough")
	close(p.quit)
	p.log.Info("Waiting for run to finish")
	<-p.done
	p.log.Info("Shutdown complete")
}

// Deposits returns a channel of processed deposits
func (p *Passthrough) Deposits() <-chan DepositInfo {
	return p.deposits
}

// processWaitDecideDeposit advances a single deposit through these states:
// StatusWaitDecide -> StatusWaitBuy
// StatusWaitBuy -> StatusWaitExecuteOrder
// StatusWaitExecuteOrder -> StatusWaitSend
func (p *Passthrough) processWaitDecideDeposit(di DepositInfo) (DepositInfo, error) {
	log := p.log.WithField("depositInfo", di)
	log.Info("Processing StatusWaitDecide deposit")

	for {
		select {
		case <-p.quit:
			return di, nil
		default:
		}

		log.Info("handleDepositInfoState")

		var err error
		di, err = p.handleDepositInfoState(di)
		log = log.WithField("depositInfo", di)

		p.setStatus(err)

		switch err.(type) {
		default:
			switch err {
			case nil:
				break
			default:
				log.WithError(err).Error("handleDepositInfoState failed")
				return di, err
			}
		}

		if di.Status == StatusWaitSend {
			return di, nil
		}
	}
}

func (p *Passthrough) handleDepositInfoState(di DepositInfo) (DepositInfo, error) {
	log := p.log.WithField("depositInfo", di)

	if err := di.ValidateForStatus(); err != nil {
		log.WithError(err).Error("handleDepositInfoState's DepositInfo is invalid")
		return di, err
	}

	switch di.Status {
	case StatusWaitDecide:
		// Set status to StatusWaitBuy
		di, err := p.store.UpdateDepositInfo(di.DepositID, func(di DepositInfo) DepositInfo {
			di.Status = StatusWaitBuy
			di.PassthroughName = PassthroughC2CX
			return di
		})
		if err != nil {
			log.WithError(err).Error("UpdateDepositInfo set StatusWaitBuy failed")
			return di, err
		}

		log.Info("DepositInfo status set to StatusWaitBuy")

		return di, nil

	case StatusWaitBuy:
		// TODO -- check balance on c2cx
		// TODO -- check balance per-order instead?
		// TODO -- maybe need a separate state machine just for purchasing
		if err := p.checkBalance(); err != nil {
			return di, err
		}

		// Set status to StatusWaitExecuteOrder
		di, err := p.store.UpdateDepositInfo(di.DepositID, func(di DepositInfo) DepositInfo {
			di.Status = StatusWaitBuy
			return di
		})
		if err != nil {
			log.WithError(err).Error("UpdateDepositInfo set StatusWaitBuy failed")
			return di, err
		}

		log.Info("DepositInfo status set to StatusWaitBuy")

		return di, nil

	case StatusWaitExecuteOrder:
		// TODO -- buy from c2cx

	default:
		err := ErrDepositStatusInvalid
		log.WithError(err).Error(err)
		return di, err
	}

	return di, nil
}

// checkBalance checks that enough coins are held on the exchange
func (p *Passthrough) checkBalance() error {
	return nil
}

func (p *Passthrough) setStatus(err error) {
	defer p.statusLock.Unlock()
	p.statusLock.Lock()
	p.status = err
}

// Status returns the last return value of the processing state
func (p *Passthrough) Status() error {
	defer p.statusLock.RUnlock()
	p.statusLock.RLock()
	return p.status
}
