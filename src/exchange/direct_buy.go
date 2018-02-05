package exchange

import (
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/skycoin/teller/src/config"
)

// Processor is a component that processes deposits from a Receiver and sends them to a Sender
type Processor interface {
	Deposits() <-chan DepositInfo
}

// ProcessRunner is a Processor that can be run
type ProcessRunner interface {
	Runner
	Processor
}

// DirectBuy implements a Processor. All deposits are sent directly to the sender for processing.
type DirectBuy struct {
	log      logrus.FieldLogger
	cfg      config.SkyExchanger
	receiver Receiver
	store    Storer
	deposits chan DepositInfo
	quit     chan struct{}
	done     chan struct{}
}

// NewDirectBuy creates DirectBuy
func NewDirectBuy(log logrus.FieldLogger, cfg config.SkyExchanger, store Storer, receiver Receiver) (*DirectBuy, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &DirectBuy{
		log:      log.WithField("prefix", "teller.exchange.directbuy"),
		cfg:      cfg,
		store:    store,
		receiver: receiver,
		deposits: make(chan DepositInfo, 100),
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
	}, nil
}

// Run updates all deposits with StatusWaitSend and exposes them over Deposits()
func (p *DirectBuy) Run() error {
	log := p.log
	log.Info("Start direct buy service...")
	defer func() {
		log.Info("Closed direct buy service")
		p.done <- struct{}{}
	}()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		p.runUpdateStatus()
	}()

	wg.Wait()

	return nil
}

// runUpdateStatus reads deposits from the Receiver and changes their status to StatusWaitSend
func (p *DirectBuy) runUpdateStatus() {
	log := p.log.WithField("goroutine", "runUpdateStatus")
	for {
		select {
		case <-p.quit:
			log.Info("quit")
			return
		case d := <-p.receiver.Deposits():
			updatedDeposit, err := p.updateStatus(d)
			if err != nil {
				msg := "updateStatus failed. This deposit will not be reprocessed until teller is restarted."
				log.WithField("depositInfo", d).WithError(err).Error(msg)
				continue
			}

			p.deposits <- updatedDeposit
		}
	}
}

// Shutdown stops a previous call to Run
func (p *DirectBuy) Shutdown() {
	p.log.Info("Shutting down DirectBuy")
	close(p.quit)
	p.log.Info("Waiting for run to finish")
	<-p.done
	p.log.Info("Shutdown complete")
}

// Deposits returns a channel of processed deposits
func (p *DirectBuy) Deposits() <-chan DepositInfo {
	return p.deposits
}

// updateStatus sets the deposit's status to StatusWaitSend.
// The deposit will be picked up by the Send component which will send the coins.
// The fixed exchange rate is already set by the receiver when it creates the deposit, so no other action is needed.
// TODO -- set the rate here instead?
func (p *DirectBuy) updateStatus(di DepositInfo) (DepositInfo, error) {
	updatedDi, err := p.store.UpdateDepositInfo(di.DepositID, func(di DepositInfo) DepositInfo {
		di.Status = StatusWaitSend
		return di
	})
	if err != nil {
		p.log.WithError(err).Error("UpdateDepositInfo set StatusWaitSend failed")
		return di, err
	}

	return updatedDi, nil
}
