package exchange

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"github.com/skycoin/exchange-api/db"
	"github.com/skycoin/exchange-api/exchange"
	c2cx "github.com/skycoin/exchange-api/exchange/c2cx.com"
	"github.com/skycoin/skycoin/src/util/droplet"
	"github.com/skycoin/skycoin/src/visor"

	"github.com/skycoin/teller/src/config"
	"github.com/skycoin/teller/src/scanner"
)

const (
	checkOrderWait = time.Second
)

// Passthrough implements a Processor. For each deposit, it buys a corresponding amount
// from c2cx.com, then tells the sender to send the amount bought.
type Passthrough struct {
	log            logrus.FieldLogger
	cfg            config.SkyExchanger
	receiver       Receiver
	store          Storer
	deposits       chan DepositInfo
	quit           chan struct{}
	done           chan struct{}
	statusLock     sync.RWMutex
	status         error
	exchangeClient exchange.Client
}

// NewPassthrough creates Passthrough
func NewPassthrough(log logrus.FieldLogger, cfg config.SkyExchanger, store Storer, receiver Receiver) (*Passthrough, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	orderbookDatabase, err := db.NewOrderbookTracker()
	if err != nil {
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
		exchangeClient: &c2cx.Client{
			Key:                      cfg.ExchangeClient.Key,
			Secret:                   cfg.ExchangeClient.Secret,
			OrdersRefreshInterval:    cfg.ExchangeClient.OrdersRefreshInterval,
			OrderbookRefreshInterval: cfg.ExchangeClient.OrderbookRefreshInterval,
			Orders:     exchange.NewTracker(),
			Orderbooks: orderbookDatabase,
		},
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
				msg := "handleDeposit failed. This deposit will not be reprocessed until teller is restarted."
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
// StatusWaitDecide -> StatusWaitPassthrough
// StatusWaitPassthrough -> StatusWaitSend
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
		// Set status to StatusWaitPassthrough
		di, err := p.store.UpdateDepositInfo(di.DepositID, func(di DepositInfo) DepositInfo {
			di.Status = StatusWaitPassthrough
			di.Passthrough.ExchangeName = PassthroughExchangeC2CX
			return di
		})
		if err != nil {
			log.WithError(err).Error("UpdateDepositInfo set StatusWaitPassthrough failed")
			return di, err
		}

		log.Info("DepositInfo status set to StatusWaitPassthrough")

		return di, nil

	case StatusWaitPassthrough:
		di, err := p.fillOrder(di)
		if err != nil {
			return di, err
		}

		// Set status to StatusWaitSend
		di, err = p.store.UpdateDepositInfo(di.DepositID, func(di DepositInfo) DepositInfo {
			di.Status = StatusWaitSend
			return di
		})
		if err != nil {
			log.WithError(err).Error("UpdateDepositInfo set StatusWaitSend failed")
			return di, err
		}

		log.Info("DepositInfo status set to StatusWaitSend")

		return di, nil

	default:
		err := ErrDepositStatusInvalid
		log.WithError(err).Error(err)
		return di, err
	}
}

// checkBalance checks that enough coins are held on the exchange
func (p *Passthrough) checkBalance(di DepositInfo) error {
	quantity, err := p.exchangeClient.GetBalance(di.CoinType)

	if err != nil {
		p.log.WithField("depositInfo", di).WithError(err).Error("Failed to get balance from exchange")
		return err
	}

	switch di.CoinType {
	case scanner.CoinTypeBTC:
		quantity = quantity.Mul(decimal.New(SatoshisPerBTC, 0))
	case scanner.CoinTypeETH:
		quantity = quantity.Mul(decimal.New(WeiPerETH, 0))
	default:
		return scanner.ErrUnsupportedCoinType
	}

	if quantity.LessThan(decimal.New(di.DepositValue, 0)) {
		return ErrLowExchangeBalance
	}

	return nil
}

// getCheapestAsk returns the cheapest ask order from c2cx
func (p *Passthrough) getCheapestAsk(di DepositInfo) (*exchange.MarketOrder, error) {
	marketRecord, err := p.exchangeClient.Orderbook().Get(fmt.Sprintf("SKY_%s", di.CoinType))
	if err != nil {
		return nil, err
	}

	marketOrder := marketRecord.CheapestAsk()
	if marketOrder == nil {
		return nil, ErrNoAsksAvailable
	}

	return marketOrder, nil
}

// placeOrder places an order on the exchange and returns the orderID
func (p *Passthrough) placeOrder(di DepositInfo, ask *exchange.MarketOrder) (int, *exchange.MarketOrder, error) {
	order, err := buildOrderFromDeposit(di, ask)
	if err != nil {
		return 0, nil, err
	}

	orderID, err := p.exchangeClient.Buy(fmt.Sprintf("SKY_%s", di.CoinType), order.Price, order.Volume)
	if err != nil {
		return 0, nil, err
	}

	return orderID, order, nil
}

// buildOrderFromDeposit creates an order given a deposit and an ask order.
// The order's volume will based upon the amount of BTC to spend on the order,
// given the fixed price of the input ask order.
func buildOrderFromDeposit(di DepositInfo, ask *exchange.MarketOrder) (*exchange.MarketOrder, error) {
	// Calculate the cost of the ask order in BTC
	// If the amount available to spend is less than the cost of the order,
	// adjust the volume of the ask order to match it.
	if di.DepositValue <= di.Passthrough.DepositValueSpent {
		return nil, errors.New("deposit has been entirely spent")
	}

	cost := askCost(ask)
	available := decimal.New(di.DepositValue-di.Passthrough.DepositValueSpent, -int32(SatoshiExponent))

	// We can afford the entire order, return it unmodified
	if available.GreaterThan(cost) {
		return ask, nil
	}

	// We will bid on part of the order, adjust the volume downward
	volume := available.Div(ask.Price)

	// Truncate the volume to the maximum decimals spendable by Skycoin
	volume = volume.Truncate(int32(visor.MaxDropletPrecision))

	return &exchange.MarketOrder{
		Price:  ask.Price,
		Volume: volume,
	}, nil
}

// Calculates the cost of an ask order
// TODO -- make this a method of exchange.MarketOrder
// exchange.MarketOrder should also specify the currency types involved
func askCost(ask *exchange.MarketOrder) decimal.Decimal {
	return ask.Price.Mul(ask.Volume)
}

// checkOrder returns the status of an order
func (p *Passthrough) checkOrder(orderID int) (string, error) {
	return p.exchangeClient.OrderStatus(orderID)
}

// clearOrders cancels all pending orders
func (p *Passthrough) clearOrders() error {
	_, err := p.exchangeClient.CancelAll()
	return err
}

// fillOrder buys one order at a time from the exchange
func (p *Passthrough) fillOrder(di DepositInfo) (DepositInfo, error) {
	// checkBalance
	// getOrderbook
	// buy cheapest order:
	//  buy entire order or partial order
	//  check for status
	//  if status does not complete in time frame, cancel
	//   check for status

	// find order
	// compare order amount to DepositValue
	//      need to track remaining DepositValue to spend from
	// Example ask BTC_SKY: [0.00189,46.49] [price in btc, sky qty]

	// TODO -- determine fatal/retry cases
	// TODO -- API wrapper around exchange-api

	log := p.log.WithField("depositInfo", di)

	if err := p.checkBalance(di); err != nil {
		log.WithError(err).Error("checkBalance failed")
		return di, err
	}

beginOrderLoop:
	for di.Passthrough.DepositValueSpent < di.DepositValue {
		// Clear any pending orders by cancelling them
		// TODO -- if any orders actually ended up completed, match the orderID
		// with the deposit that made them, and update the deposit
		// TODO -- find out what happens when cancelling an order that already completed
		if err := p.clearOrders(); err != nil {
			log.WithError(err).Error("clearOrders failed")
			return di, err
		}

		// Get the cheapest ask bid
		ask, err := p.getCheapestAsk(di)
		if err != nil {
			log.WithError(err).Error("getCheapestAsk failed")
			return di, err
		}

		// Place an order matching this ask bid
		// TODO -- adjust amount based upon remaining BTC to spend
		orderID, ask, err := p.placeOrder(di, ask)
		if err != nil {
			log.WithError(err).Error("placeOrder failed")
			return di, err
		}

		log.WithFields(logrus.Fields{
			"orderID":   orderID,
			"askPrice":  ask.Price,
			"askVolume": ask.Volume,
			"askCost":   askCost(ask),
		}).Info("Placed order")

		// Wait for order to complete
		// If not completed after checkOrderWait, cancel it
		// Wait for a final state (complete or cancelled)
		var status string
		select {
		case <-p.quit:
			return di, nil
		case <-time.After(checkOrderWait):
			status, err = p.checkOrder(orderID)
			if err != nil {
				log.WithError(err).Error("checkOrder failed")
				return di, err
			}

			log.WithFields(logrus.Fields{
				"orderID":     orderID,
				"orderStatus": status,
			}).Info("Got order status")

			switch status {
			case exchange.Completed:
			default:
				continue beginOrderLoop
			}
		}

		// Calculate the number of SKY bought, measured in droplets
		skyBought := ask.Volume.Mul(decimal.New(droplet.Multiplier, 0)).IntPart()
		if skyBought < 0 {
			err := errors.New("Calculated SKY bought is negative")
			log.WithError(err).Error("CRITICAL ERROR")
			return di, err
		}

		// Calculate number of satoshis spent
		// TODO -- review the math to make sure the truncation caused by IntPart()
		// does not lead us to have a tiny unspendable amount of BTC remaining
		// that prevents us from exiting the loop.
		// A skycoin droplet has a minimum price measured in BTC -- this may be unavoidable
		// and the loop check may have to account for this possibility.
		// Also, we only purchase skycoin to a precision that we are able to send,
		// defined by visor.MaxDropletPrecision, which is 3 at the time of this writing.
		// So the smallest purchasable unit is 0.001 SKY.
		// If the BTC/SKY price is 0.0012, then the minimum amount of BTC to spend
		// is 0.0012 * 0.001 = 0.0000012 BTC, or 120 satoshis.
		spent := askCost(ask).Mul(decimal.New(SatoshisPerBTC, 0)).IntPart()

		// Update deposit info
		// TODO -- use UpdateDepositInfoCallback, in case the DB save fails?
		di, err = p.store.UpdateDepositInfo(di.DepositID, func(di DepositInfo) DepositInfo {
			di.Passthrough.Orders = append(di.Passthrough.Orders, PassthroughOrder{})
			di.Passthrough.SkyBought += uint64(skyBought)
			di.Passthrough.DepositValueSpent += spent
			return di
		})
		if err != nil {
			log.WithError(err).Error("Attempted to save passthrough order: UpdateDepositInfo failed")
			return di, err
		}
	}

	return di, nil
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
