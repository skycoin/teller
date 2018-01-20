package exchange

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/skycoin/skycoin/src/api/cli"
	"github.com/skycoin/skycoin/src/coin"
	"github.com/skycoin/skycoin/src/util/droplet"
	"github.com/skycoin/skycoin/src/visor"

	"github.com/skycoin/teller/src/scanner"
	"github.com/skycoin/teller/src/sender"
	"github.com/skycoin/teller/src/util/mathutil"
)

const (
	// SatoshisPerBTC is the number of satoshis per 1 BTC
	SatoshisPerBTC int64 = 1e8
	// WeiPerETH is the number of wei per 1 ETH
	WeiPerETH int64 = 1e18

	txConfirmationCheckWait = time.Second * 3
)

var (
	// ErrEmptySendAmount is returned if the calculated skycoin amount to send is 0
	ErrEmptySendAmount = errors.New("Skycoin send amount is 0")
	// ErrNoResponse is returned when the send service returns a nil response. This happens if the send service has closed.
	ErrNoResponse = errors.New("No response from the send service")
	// ErrNotConfirmed is returned if the tx is not confirmed yet
	ErrNotConfirmed = errors.New("Transaction is not confirmed yet")
	// ErrDepositStatusInvalid is returned when handling a deposit with a status that cannot be processed
	// This includes StatusWaitDeposit and StatusUnknown
	ErrDepositStatusInvalid = errors.New("Deposit status cannot be handled")
	// ErrNoBoundAddress is returned if no skycoin address is bound to a deposit's address
	ErrNoBoundAddress = errors.New("Deposit has no bound skycoin address")
)

// DepositFilter filters deposits
type DepositFilter func(di DepositInfo) bool

// Exchanger provides APIs to interact with the exchange service
type Exchanger interface {
	BindAddress(skyAddr, depositAddr, coinType string) error
	GetDepositStatuses(skyAddr string) ([]DepositStatus, error)
	GetDepositStatusDetail(flt DepositFilter) ([]DepositStatusDetail, error)
	GetBindNum(skyAddr string) (int, error)
	GetDepositStats() (*DepositStats, error)
	Status() error
	Balance() (*cli.Balance, error)
}

// Exchange manages coin exchange between deposits and skycoin
type Exchange struct {
	log         logrus.FieldLogger
	cfg         Config
	multiplexer *scanner.Multiplexer // multiplex provides APIs for interacting with the scan service
	sender      sender.Sender        // sender provides APIs for sending skycoin
	store       Storer               // deposit info storage
	quit        chan struct{}
	done        chan struct{}
	depositChan chan DepositInfo
	statusLock  sync.RWMutex
	status      error
}

// Config exchange config struct
type Config struct {
	BtcRate                 string // SKY/BTC rate, decimal string
	EthRate                 string // SKY/ETH rate, decimal string
	TxConfirmationCheckWait time.Duration
	MaxDecimals             int
}

// Validate returns an error if the configuration is invalid
func (c Config) Validate() error {
	if _, err := ParseRate(c.BtcRate); err != nil {
		return err
	}

	if c.MaxDecimals < 0 {
		return errors.New("MaxDecimals can't be negative")
	}

	if uint64(c.MaxDecimals) > visor.MaxDropletPrecision {
		return fmt.Errorf("MaxDecimals is larger than visor.MaxDropletPrecision=%d", visor.MaxDropletPrecision)
	}

	return nil
}

// NewExchange creates exchange service
func NewExchange(log logrus.FieldLogger, store Storer, multiplexer *scanner.Multiplexer, sender sender.Sender, cfg Config) (*Exchange, error) {
	if _, err := ParseRate(cfg.BtcRate); err != nil {
		return nil, err
	}

	if cfg.TxConfirmationCheckWait == 0 {
		cfg.TxConfirmationCheckWait = txConfirmationCheckWait
	}

	return &Exchange{
		cfg:         cfg,
		log:         log.WithField("prefix", "teller.exchange"),
		multiplexer: multiplexer,
		sender:      sender,
		store:       store,
		quit:        make(chan struct{}),
		done:        make(chan struct{}, 1),
		depositChan: make(chan DepositInfo, 100),
	}, nil
}

// Run starts the exchange process
func (s *Exchange) Run() error {
	log := s.log
	log.Info("Start exchange service...")
	defer func() {
		log.Info("Closed exchange service")
		s.done <- struct{}{}
	}()

	// Load StatusWaitSend deposits for processing later
	waitSendDeposits, err := s.store.GetDepositInfoArray(func(di DepositInfo) bool {
		return di.Status == StatusWaitSend
	})

	if err != nil {
		err = fmt.Errorf("GetDepositInfoArray failed: %v", err)
		log.WithError(err).Error(err)
		return err
	}

	// Load StatusWaitConfirm deposits for processing later
	waitConfirmDeposits, err := s.store.GetDepositInfoArray(func(di DepositInfo) bool {
		return di.Status == StatusWaitConfirm
	})

	if err != nil {
		err = fmt.Errorf("GetDepositInfoArray failed: %v", err)
		log.WithError(err).Error(err)
		return err
	}

	var wg sync.WaitGroup

	// This loop processes StatusWaitSend deposits.
	// Only one deposit is processed at a time; it will not send more coins
	// until it receives confirmation of the previous send.
	wg.Add(1)
	go func() {
		defer wg.Done()

		log := log.WithField("goroutine", "sendSky")
		for {
			select {
			case <-s.quit:
				log.Info("exchange.Exchange send loop quit")
				return
			case d := <-s.depositChan:
				log := log.WithField("depositInfo", d)
				if err := s.processWaitSendDeposit(d); err != nil {
					log.WithError(err).Error("processWaitSendDeposit failed. This deposit will not be reprocessed until teller is restarted.")
				}
			}
		}
	}()

	// Queue the saved StatusWaitConfirm deposits
	for _, di := range waitConfirmDeposits {
		s.depositChan <- di
	}

	// Queue the saved StatusWaitSend deposits
	for _, di := range waitSendDeposits {
		s.depositChan <- di
	}

	// This loop processes incoming deposits from the scanner and saves a
	// new DepositInfo with a status of StatusWaitSend
	wg.Add(1)
	go func() {
		defer wg.Done()

		log := log.WithField("goroutine", "watchDeposits")
		for {
			var dv scanner.DepositNote
			var ok bool
			select {
			case <-s.quit:
				log.Info("exchange.Exchange watch deposits loop quit")
				return
			case dv, ok = <-s.multiplexer.GetDeposit():
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
			if d, err := s.saveIncomingDeposit(dv.Deposit); err != nil {
				log.WithError(err).Error("saveIncomingDeposit failed. This deposit will not be reprocessed until teller is restarted.")
				dv.ErrC <- err
			} else {
				dv.ErrC <- nil
				s.depositChan <- d
			}
		}
	}()

	wg.Wait()

	return nil
}

// Shutdown close the exchange service
func (s *Exchange) Shutdown() {
	close(s.quit)
	s.log.Info("Waiting for Run() to finish")
	<-s.done
	s.log.Info("Shutdown complete")
}

//getRate returns conversion rate according to coin type
func (s *Exchange) getRate(coinType string) (string, error) {
	switch coinType {
	case scanner.CoinTypeBTC:
		s.log.Info("Received bitcoin deposit")
		return s.cfg.BtcRate, nil
	case scanner.CoinTypeETH:
		s.log.Info("Received ethcoin deposit")
		return s.cfg.EthRate, nil
	default:
		s.log.WithError(scanner.ErrUnsupportedCoinType).Error()
		return "", scanner.ErrUnsupportedCoinType
	}
}

// saveIncomingDeposit is called when receiving a deposit from the scanner
func (s *Exchange) saveIncomingDeposit(dv scanner.Deposit) (DepositInfo, error) {
	log := s.log.WithField("deposit", dv)

	var rate string
	rate, err := s.getRate(dv.CoinType)
	if err != nil {
		log.WithError(err).Error("get conversion rate failed")
		return DepositInfo{}, err
	}

	di, err := s.store.GetOrCreateDepositInfo(dv, rate)
	if err != nil {
		log.WithError(err).Error("GetOrCreateDepositInfo failed")
		return DepositInfo{}, err
	}

	log = log.WithField("depositInfo", di)
	log.Info("Saved DepositInfo")

	return di, err
}

// processDeposit advances a single deposit through three states:
// StatusWaitSend -> StatusWaitConfirm
// StatusWaitConfirm -> StatusDone
// StatusWaitDeposit is never saved to the database, so it does not transition
func (s *Exchange) processWaitSendDeposit(di DepositInfo) error {
	log := s.log.WithField("depositInfo", di)
	log.Info("Processing StatusWaitSend deposit")

	for {
		select {
		case <-s.quit:
			return nil
		default:
		}

		log.Info("handleDepositInfoState")

		var err error
		di, err = s.handleDepositInfoState(di)
		log = log.WithField("depositInfo", di)

		s.setStatus(err)

		switch err.(type) {
		case sender.RPCError:
			// Treat skycoin RPC/CLI errors as temporary.
			// Some RPC/CLI errors are hypothetically permanent,
			// but most likely it is an insufficient wallet balance or
			// the skycoin node is unavailable.
			// A permanent error suggests a bug in skycoin or teller so can be fixed.
			log.WithError(err).Error("handleDepositInfoState failed")
			select {
			case <-time.After(s.cfg.TxConfirmationCheckWait):
			case <-s.quit:
				return nil
			}
		default:
			switch err {
			case nil:
				break
			case ErrNotConfirmed:
				select {
				case <-time.After(s.cfg.TxConfirmationCheckWait):
				case <-s.quit:
					return nil
				}
			default:
				log.WithError(err).Error("handleDepositInfoState failed")
				return err
			}
		}

		if di.Status == StatusDone {
			return nil
		}
	}

	return nil
}

func (s *Exchange) handleDepositInfoState(di DepositInfo) (DepositInfo, error) {
	log := s.log.WithField("deposit", di)

	if err := di.ValidateForStatus(); err != nil {
		log.WithError(err).Error("handleDepositInfoState's DepositInfo is invalid")
		return di, err
	}

	switch di.Status {
	case StatusWaitSend:
		// Prepare skycoin transaction
		skyTx, err := s.createTransaction(di)

		if err != nil {
			log.WithError(err).Error("createTransaction failed")

			// If the send amount is empty, skip to StatusDone.
			if err == ErrEmptySendAmount {
				log.Info("Send amount is 0, skipping to StatusDone")
				di, err = s.store.UpdateDepositInfo(di.DepositID, func(di DepositInfo) DepositInfo {
					di.Status = StatusDone
					di.Error = ErrEmptySendAmount.Error()
					return di
				})
				if err != nil {
					log.WithError(err).Error("Update DepositInfo set StatusDone failed")
					return di, err
				}

				log.WithError(ErrEmptySendAmount).Info("DepositInfo set to StatusDone")

				return di, nil
			}

			return di, err
		}

		// Find the coins from the skyTx
		// The skyTx contains one output sent to the destination address,
		// so this check is safe.
		// It is verified earlier by verifyCreatedTransaction
		var skySent uint64
		for _, o := range skyTx.Out {
			if o.Address.String() == di.SkyAddress {
				skySent = o.Coins
				break
			}
		}

		if skySent == 0 {
			err := errors.New("No output to destination address found in transaction")
			log.WithError(err).Error(err)
			return di, err
		}

		// Within a bolt.DB transaction, update the db then send the coins
		// If the send fails, the data is rolled back
		// If the db save fails, no coins had been sent
		di, err = s.store.UpdateDepositInfoCallback(di.DepositID, func(di DepositInfo) DepositInfo {
			di.Status = StatusWaitConfirm
			di.Txid = skyTx.TxIDHex()
			di.SkySent = skySent
			return di
		}, func(di DepositInfo) error {
			// NOTE: broadcastTransaction retries indefinitely on error
			// If the skycoin node is not reachable, this will block,
			// which will also block the database since it's in a transaction
			rsp, err := s.broadcastTransaction(skyTx)
			if err != nil {
				log.WithError(err).Error("broadcastTransaction failed")
				return err
			}

			// Invariant assertion: do not return this as an error, since
			// coins have been sent. This should never occur.
			if rsp.Txid != skyTx.TxIDHex() {
				log.Error("CRITICAL ERROR: BroadcastTxResponse.Txid != skyTx.TxIDHex()")
			}

			return nil
		})

		if err != nil {
			log.WithError(err).Error("store.UpdateDepositInfoCallback failed")
			return di, err
		}

		log.Info("DepositInfo set to StatusWaitConfirm")

		return di, nil

	case StatusWaitConfirm:
		// Wait for confirmation
		rsp := s.sender.IsTxConfirmed(di.Txid)

		if rsp == nil {
			log.WithError(ErrNoResponse).Warn("Sender closed")
			return di, ErrNoResponse
		}

		if rsp.Err != nil {
			log.WithError(rsp.Err).Error("IsTxConfirmed failed")
			return di, rsp.Err
		}

		if !rsp.Confirmed {
			log.Info("Transaction is not confirmed yet")
			return di, ErrNotConfirmed
		}

		log.Info("Transaction is confirmed")

		di, err := s.store.UpdateDepositInfo(di.DepositID, func(di DepositInfo) DepositInfo {
			di.Status = StatusDone
			return di
		})
		if err != nil {
			log.WithError(err).Error("UpdateDepositInfo set StatusDone failed")
			return di, err
		}

		log.Info("DepositInfo status set to StatusDone")

		return di, nil

	case StatusDone:
		log.Warn("DepositInfo already processed")
		return di, nil

	case StatusWaitDeposit:
		// We don't save any deposits with StatusWaitDeposit.
		// We can't transition to StatusWaitSend without a scanner.Deposit
		log.Error("StatusWaitDeposit cannot be processed and should never be handled by this method")
		fallthrough
	case StatusUnknown:
		fallthrough
	default:
		err := ErrDepositStatusInvalid
		log.WithError(err).Error(err)
		return di, err
	}
}

func (s *Exchange) calculateSkyDroplets(di DepositInfo) (uint64, error) {
	log := s.log
	var err error
	var skyAmt uint64
	switch di.CoinType {
	case scanner.CoinTypeBTC:
		skyAmt, err = CalculateBtcSkyValue(di.DepositValue, di.ConversionRate, s.cfg.MaxDecimals)
		if err != nil {
			log.WithError(err).Error("CalculateBtcSkyValue failed")
			return 0, err
		}
	case scanner.CoinTypeETH:
		//Gwei convert to wei, because stored-value is Gwei in case overflow of uint64
		skyAmt, err = CalculateEthSkyValue(mathutil.Gwei2Wei(di.DepositValue), di.ConversionRate, s.cfg.MaxDecimals)
		if err != nil {
			log.WithError(err).Error("CalculateEthSkyValue failed")
			return 0, err
		}
	default:
		log.WithError(scanner.ErrUnsupportedCoinType).Error()
		return 0, scanner.ErrUnsupportedCoinType
	}
	return skyAmt, nil
}

func (s *Exchange) createTransaction(di DepositInfo) (*coin.Transaction, error) {
	log := s.log.WithField("deposit", di)

	// This should never occur, the DepositInfo is saved with a SkyAddress
	// during GetOrCreateDepositInfo().
	if di.SkyAddress == "" {
		err := ErrNoBoundAddress
		log.WithError(err).Error(err)
		return nil, err
	}

	log = log.WithField("skyAddr", di.SkyAddress)
	log = log.WithField("skyRate", di.ConversionRate)
	log = log.WithField("maxDecimals", s.cfg.MaxDecimals)

	skyAmt, err := s.calculateSkyDroplets(di)
	if err != nil {
		log.WithError(err).Error("calculateSkyDroplets failed")
		return nil, err
	}
	skyAmtCoins, err := droplet.ToString(skyAmt)
	if err != nil {
		log.WithError(err).Error("droplet.ToString failed")
		return nil, err
	}

	log = log.WithField("sendAmtDroplets", skyAmt)
	log = log.WithField("sendAmtCoins", skyAmtCoins)

	log.Info("Creating skycoin transaction")

	if skyAmt == 0 {
		err := ErrEmptySendAmount
		log.WithError(err).Error(err)
		return nil, err
	}

	tx, err := s.sender.CreateTransaction(di.SkyAddress, skyAmt)
	if err != nil {
		log.WithError(err).Error("sender.CreateTransaction failed")
		return nil, err
	}

	log = log.WithField("transactionOutput", tx.Out)

	if err := verifyCreatedTransaction(tx, di, skyAmt); err != nil {
		log.WithError(err).Error("verifyCreatedTransaction failed")
		return nil, err
	}

	return tx, nil
}

func verifyCreatedTransaction(tx *coin.Transaction, di DepositInfo, skyAmt uint64) error {
	// Check invariant assertions:
	// The transaction should contain one output to the destination address.
	// It may or may not have a change output.
	count := 0

	for _, o := range tx.Out {
		if o.Address.String() != di.SkyAddress {
			continue
		}

		count++

		if o.Coins != skyAmt {
			return errors.New("CreateTransaction transaction coins are different")
		}
	}

	if count == 0 {
		return fmt.Errorf("CreateTransaction transaction has no output to address %s", di.SkyAddress)
	} else if count > 1 {
		return fmt.Errorf("CreateTransaction transaction has multiple outputs to address %s", di.SkyAddress)
	}

	return nil
}

func (s *Exchange) broadcastTransaction(tx *coin.Transaction) (*sender.BroadcastTxResponse, error) {
	log := s.log.WithField("txid", tx.TxIDHex())

	log.Info("Broadcasting skycoin transaction")

	rsp := s.sender.BroadcastTransaction(tx)

	log = log.WithField("sendRsp", rsp)

	if rsp == nil {
		err := ErrNoResponse
		log.WithError(err).Warn("Sender closed")
		return nil, err
	}

	if rsp.Err != nil {
		err := fmt.Errorf("Send skycoin failed: %v", rsp.Err)
		log.WithError(err).Error(err)
		return nil, err
	}

	log.Info("Sent skycoin")

	return rsp, nil
}

// BindAddress binds deposit address with skycoin address, and
// add the btc/eth address to scan service, when detect deposit coin
// to the btc/eth address, will send specific skycoin to the binded
// skycoin address
func (s *Exchange) BindAddress(skyAddr, depositAddr, coinType string) error {
	if err := s.multiplexer.ValidateCoinType(coinType); err != nil {
		return err
	}

	if err := s.store.BindAddress(skyAddr, depositAddr, coinType); err != nil {
		return err
	}

	return s.multiplexer.AddScanAddress(depositAddr, coinType)
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
	SkyAddress     string `json:"skycoin_address"`
	DepositAddress string `json:"deposit_address"`
	CoinType       string `json:"coin_type"`
	Txid           string `json:"txid"`
}

// GetDepositStatuses returns deamon.DepositStatus array of given skycoin address
func (s *Exchange) GetDepositStatuses(skyAddr string) ([]DepositStatus, error) {
	dis, err := s.store.GetDepositInfoOfSkyAddress(skyAddr)
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
func (s *Exchange) GetDepositStatusDetail(flt DepositFilter) ([]DepositStatusDetail, error) {
	dis, err := s.store.GetDepositInfoArray(flt)
	if err != nil {
		return nil, err
	}

	dss := make([]DepositStatusDetail, 0, len(dis))
	for _, di := range dis {
		dss = append(dss, DepositStatusDetail{
			Seq:            di.Seq,
			UpdatedAt:      di.UpdatedAt,
			Status:         di.Status.String(),
			SkyAddress:     di.SkyAddress,
			DepositAddress: di.DepositAddress,
			Txid:           di.Txid,
			CoinType:       di.CoinType,
		})
	}
	return dss, nil
}

// GetBindNum returns the number of btc/eth address the given sky address binded
func (s *Exchange) GetBindNum(skyAddr string) (int, error) {
	addrs, err := s.store.GetSkyBindAddresses(skyAddr)
	return len(addrs), err
}

// GetDepositStats returns deposit status
func (s *Exchange) GetDepositStats() (*DepositStats, error) {
	tbr, tss, err := s.store.GetDepositStats()
	if err != nil {
		return nil, err
	}

	return &DepositStats{
		TotalBTCReceived: tbr,
		TotalSKYSent:     tss,
	}, nil
}

// Balance returns the number of coins left in the OTC wallet
func (s *Exchange) Balance() (*cli.Balance, error) {
	return s.sender.Balance()
}

func (s *Exchange) setStatus(err error) {
	defer s.statusLock.Unlock()
	s.statusLock.Lock()
	s.status = err
}

// Status returns the last return value of the processing state
func (s *Exchange) Status() error {
	defer s.statusLock.RUnlock()
	s.statusLock.RLock()
	return s.status
}
