package exchange

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"

	"time"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
	logrus_test "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/skycoin/src/api/cli"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/skycoin/src/coin"

	"github.com/skycoin/teller/src/config"
	"github.com/skycoin/teller/src/scanner"
	"github.com/skycoin/teller/src/sender"
	"github.com/skycoin/teller/src/util/testutil"
)

type dummySender struct {
	sync.RWMutex
	createTransactionErr    error
	broadcastTransactionErr error
	confirmErr              error
	txidConfirmMap          map[string]bool
	changeAddr              string
	changeCoins             uint64
}

func newDummySender() *dummySender {
	return &dummySender{
		txidConfirmMap: make(map[string]bool),
		changeAddr:     "nYTKxHm6SZWAMdDVx6U9BqxKMuCjmSLp93",
		changeCoins:    111e6,
	}
}

func (s *dummySender) CreateTransaction(destAddr string, coins uint64) (*coin.Transaction, error) {
	if s.createTransactionErr != nil {
		return nil, s.createTransactionErr
	}

	addr := cipher.MustDecodeBase58Address(destAddr)
	changeAddr := cipher.MustDecodeBase58Address(s.changeAddr)

	return &coin.Transaction{
		Out: []coin.TransactionOutput{
			{
				Address: changeAddr,
				Coins:   s.changeCoins,
			},
			{
				Address: addr,
				Coins:   coins,
			},
		},
	}, nil
}

func (s *dummySender) BroadcastTransaction(tx *coin.Transaction) *sender.BroadcastTxResponse {
	req := sender.BroadcastTxRequest{
		Tx:   tx,
		RspC: make(chan *sender.BroadcastTxResponse, 1),
	}

	if s.broadcastTransactionErr != nil {
		return &sender.BroadcastTxResponse{
			Err: s.broadcastTransactionErr,
			Req: req,
		}
	}

	return &sender.BroadcastTxResponse{
		Txid: tx.TxIDHex(),
		Req:  req,
	}
}

func (s *dummySender) IsTxConfirmed(txid string) *sender.ConfirmResponse {
	s.RLock()
	defer s.RUnlock()

	req := sender.ConfirmRequest{
		Txid: txid,
	}

	if s.confirmErr != nil {
		return &sender.ConfirmResponse{
			Err: s.confirmErr,
			Req: req,
		}
	}

	confirmed := s.txidConfirmMap[txid]
	return &sender.ConfirmResponse{
		Confirmed: confirmed,
		Req:       req,
	}
}

func (s *dummySender) predictTxid(t *testing.T, destAddr string, coins uint64) string {
	tx, err := s.CreateTransaction(destAddr, coins)
	require.NoError(t, err)
	return tx.TxIDHex()
}

func (s *dummySender) setTxConfirmed(txid string) {
	s.Lock()
	defer s.Unlock()

	s.txidConfirmMap[txid] = true
}

func (s *dummySender) Balance() (*cli.Balance, error) {
	return &cli.Balance{
		Coins: "100.000000",
		Hours: "100",
	}, nil
}

type dummyScanner struct {
	dvC   chan scanner.DepositNote
	addrs []string
}

func newDummyScanner() *dummyScanner {
	return &dummyScanner{
		dvC: make(chan scanner.DepositNote, 10),
	}
}

func (scan *dummyScanner) AddScanAddress(btcAddr, coinType string) error {
	scan.addrs = append(scan.addrs, btcAddr)
	return nil
}

func (scan *dummyScanner) GetDeposit() <-chan scanner.DepositNote {
	return scan.dvC
}

func (scan *dummyScanner) GetScanAddresses() ([]string, error) {
	return []string{}, nil
}

func (scan *dummyScanner) addDeposit(d scanner.DepositNote) {
	scan.dvC <- d
}

func (scan *dummyScanner) stop() {
	close(scan.dvC)
}

const (
	testSkyBtcRate      = "100" // 100 SKY per BTC
	testSkyEthRate      = "10"  // 10 SKY per ETH
	testMaxDecimals     = 0
	testSkyAddr         = "2Wbi4wvxC4fkTYMsS2f6HaFfW4pafDjXcQW"
	testSkyAddr2        = "hs1pyuNgxDLyLaZsnqzQG9U3DKdJsbzNpn"
	testWalletFile      = "test.wlt"
	dbScanTimeout       = time.Second * 3
	statusCheckTimeout  = time.Second * 3
	statusCheckInterval = time.Millisecond * 10
	statusCheckNilWait  = time.Second
	dbCheckWaitTime     = time.Millisecond * 300
)

var (
	defaultCfg = config.SkyExchanger{
		BuyMethod:               config.BuyMethodDirect,
		SkyBtcExchangeRate:      testSkyBtcRate,
		SkyEthExchangeRate:      testSkyEthRate,
		TxConfirmationCheckWait: time.Millisecond * 100,
		Wallet:                  testWalletFile,
		SendEnabled:             true,
	}
)

func newTestExchange(t *testing.T, log *logrus.Logger, db *bolt.DB) *Exchange {
	store, err := NewStore(log, db)
	require.NoError(t, err)

	bscr := newDummyScanner()
	escr := newDummyScanner()
	multiplexer := scanner.NewMultiplexer(log)
	err = multiplexer.AddScanner(bscr, scanner.CoinTypeBTC)
	require.NoError(t, err)
	err = multiplexer.AddScanner(escr, scanner.CoinTypeETH)
	require.NoError(t, err)

	go testutil.CheckError(t, multiplexer.Multiplex)

	e, err := NewDirectExchange(log, defaultCfg, store, multiplexer, newDummySender())
	require.NoError(t, err)
	return e
}

func setupExchange(t *testing.T, log *logrus.Logger) (*Exchange, func(), func()) {
	db, shutdownDB := testutil.PrepareDB(t)

	e := newTestExchange(t, log, db)

	done := make(chan struct{})
	run := func() {
		err := e.Run()
		require.NoError(t, err)
		close(done)
	}

	shutdown := func() {
		shutdownDB()
		<-done
	}

	return e, run, shutdown
}

func closeMultiplexer(e *Exchange) {
	mp := e.Receiver.(*Receive).multiplexer
	mp.GetScanner(scanner.CoinTypeBTC).(*dummyScanner).stop()
	mp.GetScanner(scanner.CoinTypeETH).(*dummyScanner).stop()
	mp.Shutdown()
}

func runExchange(t *testing.T) (*Exchange, func(), *logrus_test.Hook) {
	log, hook := testutil.NewLogger(t)
	e, run, shutdown := setupExchange(t, log)
	go run()
	return e, shutdown, hook
}

func runExchangeMockStore(t *testing.T) (*Exchange, func(), *logrus_test.Hook) {
	store := &MockStore{}
	log, hook := testutil.NewLogger(t)

	bscr := newDummyScanner()
	escr := newDummyScanner()
	multiplexer := scanner.NewMultiplexer(log)
	err := multiplexer.AddScanner(bscr, scanner.CoinTypeBTC)
	require.NoError(t, err)
	err = multiplexer.AddScanner(escr, scanner.CoinTypeETH)
	require.NoError(t, err)

	go testutil.CheckError(t, multiplexer.Multiplex)

	e, err := NewDirectExchange(log, defaultCfg, store, multiplexer, newDummySender())
	require.NoError(t, err)

	done := make(chan struct{})
	run := func() {
		err := e.Run()
		require.NoError(t, err)
		close(done)
	}
	go run()

	shutdown := func() {
		<-done
	}

	return e, shutdown, hook
}

func checkExchangerStatus(t *testing.T, e Exchanger, expectedErr error) {
	// When the expected error is nil, we can only wait a period of time to
	// hope that an error did not appear
	if expectedErr == nil {
		time.Sleep(statusCheckNilWait)
		require.NoError(t, e.Status())
		return
	}

	// When the expected error is not nil, we can wait to see if the error
	// appears, and check that the error matches
	timeoutTicker := time.After(statusCheckTimeout)
loop:
	for {
		select {
		case <-time.Tick(statusCheckInterval):
			err := e.Status()
			if err == nil {
				continue loop
			}
			require.Equal(t, expectedErr, err)
			break loop
		case <-timeoutTicker:
			t.Fatal("Deposit status checking timed out")
			return
		}
	}
}

func TestExchangeRunShutdown(t *testing.T) {
	// Tests a simple start and stop, with no scanner activity
	e, shutdown, _ := runExchange(t)
	defer shutdown()
	defer e.Shutdown()
	closeMultiplexer(e)
}

func TestExchangeRunScannerClosed(t *testing.T) {
	// Tests that there is no problem when the scanner closes
	e, shutdown, _ := runExchange(t)
	defer shutdown()
	defer e.Shutdown()
	closeMultiplexer(e)
}

func TestExchangeRunSend(t *testing.T) {
	e, shutdown, _ := runExchange(t)
	defer shutdown()
	defer e.Shutdown()

	skyAddr := testSkyAddr
	btcAddr := "foo-btc-addr"
	mustBindAddress(t, e.store, skyAddr, btcAddr)

	var value int64 = 1e8
	skySent, err := CalculateBtcSkyValue(value, testSkyBtcRate, testMaxDecimals)
	require.NoError(t, err)
	txid := e.Sender.(*Send).sender.(*dummySender).predictTxid(t, skyAddr, skySent)

	dn := scanner.DepositNote{
		Deposit: scanner.Deposit{
			CoinType: scanner.CoinTypeBTC,
			Address:  btcAddr,
			Value:    value,
			Height:   20,
			Tx:       "foo-tx",
			N:        2,
		},
		ErrC: make(chan error, 1),
	}
	mp := e.Receiver.(*Receive).multiplexer
	mp.GetScanner(scanner.CoinTypeBTC).(*dummyScanner).addDeposit(dn)

	// First loop calls saveIncomingDeposit
	// nil is written to ErrC after this method finishes
	err = <-dn.ErrC
	require.NoError(t, err)

	// Second loop calls processWaitSendDeposit
	// It sends the coins, then confirms them

	// Periodically check the database until we observe the sent deposit
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range time.Tick(dbCheckWaitTime) {
			di, err := e.store.(*Store).getDepositInfo(dn.Deposit.ID())
			log.Printf("loop getDepositInfo %v %v\n", di, err)
			require.NoError(t, err)

			if di.Status == StatusWaitConfirm {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(dbScanTimeout):
		t.Fatal("Waiting for sent deposit timed out")
	}

	// Check DepositInfo
	di, err := e.store.(*Store).getDepositInfo(dn.Deposit.ID())
	require.NoError(t, err)

	require.NotEmpty(t, di.UpdatedAt)

	expectedDeposit := DepositInfo{
		Seq:            1,
		CoinType:       scanner.CoinTypeBTC,
		UpdatedAt:      di.UpdatedAt,
		Status:         StatusWaitConfirm,
		SkyAddress:     skyAddr,
		DepositAddress: dn.Deposit.Address,
		DepositID:      dn.Deposit.ID(),
		Txid:           txid,
		SkySent:        100e6,
		BuyMethod:      config.BuyMethodDirect,
		ConversionRate: testSkyBtcRate,
		DepositValue:   dn.Deposit.Value,
		Deposit:        dn.Deposit,
	}

	require.Equal(t, expectedDeposit, di)

	// Mark the deposit as confirmed
	e.Sender.(*Send).sender.(*dummySender).setTxConfirmed(txid)

	// Periodically check the database until we observe the confirmed deposit
	done = make(chan struct{})
	go func() {
		defer close(done)
		for range time.Tick(dbCheckWaitTime) {
			di, err := e.store.(*Store).getDepositInfo(dn.Deposit.ID())
			require.NoError(t, err)

			if di.Status == StatusDone {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(dbScanTimeout):
		t.Fatal("Waiting for confirmed deposit timed out")
	}

	checkExchangerStatus(t, e, nil)

	// Check DepositInfo
	di, err = e.store.(*Store).getDepositInfo(dn.Deposit.ID())
	require.NoError(t, err)

	require.NotEmpty(t, di.UpdatedAt)

	expectedDeposit = DepositInfo{
		Seq:            1,
		CoinType:       scanner.CoinTypeBTC,
		UpdatedAt:      di.UpdatedAt,
		Status:         StatusDone,
		SkyAddress:     skyAddr,
		DepositAddress: dn.Deposit.Address,
		DepositID:      dn.Deposit.ID(),
		Txid:           txid,
		SkySent:        100e6,
		BuyMethod:      config.BuyMethodDirect,
		ConversionRate: testSkyBtcRate,
		DepositValue:   dn.Deposit.Value,
		Deposit:        dn.Deposit,
	}

	require.Equal(t, expectedDeposit, di)
	closeMultiplexer(e)
}

func TestExchangeUpdateBroadcastTxFailure(t *testing.T) {
	// Test that a BroadcastTransaction error is handled properly
	// The DepositInfo should not be updated if BroadcastTransaction fails.
	// Test that we save the rate when first creating, not on send
	// To do this, force the sender to return errors to block the transition to
	// StatusWaitConfirm
	e, shutdown, _ := runExchange(t)
	defer shutdown()
	defer e.Shutdown()

	skyAddr := testSkyAddr
	btcAddr := "foo-btc-addr"
	mustBindAddress(t, e.store, skyAddr, btcAddr)

	// Force sender to return a broadcast tx error so that the deposit stays at StatusWaitSend
	broadcastTransactionErr := errors.New("fake broadcast transaction error")
	e.Sender.(*Send).sender.(*dummySender).broadcastTransactionErr = broadcastTransactionErr

	dn := scanner.DepositNote{
		Deposit: scanner.Deposit{
			CoinType: scanner.CoinTypeBTC,
			Address:  btcAddr,
			Value:    1e8,
			Height:   20,
			Tx:       "foo-tx",
			N:        2,
		},
		ErrC: make(chan error, 1),
	}
	mp := e.Receiver.(*Receive).multiplexer
	mp.GetScanner(scanner.CoinTypeBTC).(*dummyScanner).addDeposit(dn)

	// First loop calls saveIncomingDeposit
	// nil is written to ErrC after this method finishes
	err := <-dn.ErrC
	require.NoError(t, err)

	// Wait for the deposit status error to update
	checkExchangerStatus(t, e, fmt.Errorf("Send skycoin failed: %v", broadcastTransactionErr))

	// Check the DepositInfo in the database
	// Sky should not be sent
	di, err := e.store.(*Store).getDepositInfo(dn.Deposit.ID())
	require.NoError(t, err)
	require.NotEmpty(t, di.UpdatedAt)
	require.Equal(t, DepositInfo{
		Seq:            1,
		CoinType:       scanner.CoinTypeBTC,
		UpdatedAt:      di.UpdatedAt,
		SkyAddress:     skyAddr,
		DepositAddress: btcAddr,
		DepositID:      dn.Deposit.ID(),
		Status:         StatusWaitSend,
		BuyMethod:      config.BuyMethodDirect,
		ConversionRate: testSkyBtcRate,
		DepositValue:   dn.Deposit.Value,
		Deposit:        dn.Deposit,
	}, di)
}

func TestExchangeCreateTxFailure(t *testing.T) {
	// Test that a CreateTransaction error is handled properly
	// Test that we save the rate when first creating, not on send
	// To do this, force the sender to return errors to block the transition to
	// StatusWaitConfirm
	e, shutdown, _ := runExchange(t)
	defer shutdown()
	defer e.Shutdown()

	skyAddr := testSkyAddr
	btcAddr := "foo-btc-addr"
	mustBindAddress(t, e.store, skyAddr, btcAddr)

	// Force sender to return a create tx error so that the deposit stays at StatusWaitSend
	createTransactionErr := errors.New("fake create transaction error")
	e.Sender.(*Send).sender.(*dummySender).createTransactionErr = createTransactionErr

	dn := scanner.DepositNote{
		Deposit: scanner.Deposit{
			CoinType: scanner.CoinTypeBTC,
			Address:  btcAddr,
			Value:    1e8,
			Height:   20,
			Tx:       "foo-tx",
			N:        2,
		},
		ErrC: make(chan error, 1),
	}
	mp := e.Receiver.(*Receive).multiplexer
	mp.GetScanner(scanner.CoinTypeBTC).(*dummyScanner).addDeposit(dn)

	// First loop calls saveIncomingDeposit
	// nil is written to ErrC after this method finishes
	err := <-dn.ErrC
	require.NoError(t, err)

	// Wait for the deposit status error to update
	checkExchangerStatus(t, e, createTransactionErr)

	// Check the DepositInfo in the database
	di, err := e.store.(*Store).getDepositInfo(dn.Deposit.ID())
	require.NoError(t, err)
	require.NotEmpty(t, di.UpdatedAt)
	require.Equal(t, DepositInfo{
		Seq:            1,
		CoinType:       scanner.CoinTypeBTC,
		UpdatedAt:      di.UpdatedAt,
		SkyAddress:     skyAddr,
		DepositAddress: btcAddr,
		DepositID:      dn.Deposit.ID(),
		Status:         StatusWaitSend,
		ConversionRate: testSkyBtcRate,
		BuyMethod:      config.BuyMethodDirect,
		DepositValue:   dn.Deposit.Value,
		Deposit:        dn.Deposit,
	}, di)

	require.Error(t, e.Status())
}

func TestExchangeTxConfirmFailure(t *testing.T) {
	e, shutdown, _ := runExchange(t)
	defer shutdown()
	defer e.Shutdown()

	skyAddr := testSkyAddr
	btcAddr := "foo-btc-addr"
	mustBindAddress(t, e.store, skyAddr, btcAddr)

	var value int64 = 1e8
	skySent, err := CalculateBtcSkyValue(value, testSkyBtcRate, testMaxDecimals)
	require.NoError(t, err)
	txid := e.Sender.(*Send).sender.(*dummySender).predictTxid(t, skyAddr, skySent)

	// Force sender to return a confirm error so that the deposit stays at StatusWaitConfirm
	confirmErr := errors.New("fake confirm error")
	e.Sender.(*Send).sender.(*dummySender).confirmErr = confirmErr

	dn := scanner.DepositNote{
		Deposit: scanner.Deposit{
			CoinType: scanner.CoinTypeBTC,
			Address:  btcAddr,
			Value:    value,
			Height:   20,
			Tx:       "foo-tx",
			N:        2,
		},
		ErrC: make(chan error, 1),
	}
	mp := e.Receiver.(*Receive).multiplexer
	mp.GetScanner(scanner.CoinTypeBTC).(*dummyScanner).addDeposit(dn)

	// First loop calls saveIncomingDeposit
	// nil is written to ErrC after this method finishes
	err = <-dn.ErrC
	require.NoError(t, err)

	// Wait for the deposit status error to update
	checkExchangerStatus(t, e, confirmErr)

	// Wait for StatusWaitSend deposit in the database
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range time.Tick(dbCheckWaitTime) {
			// Check the DepositInfo in the database
			di, err := e.store.(*Store).getDepositInfo(dn.Deposit.ID())
			require.NoError(t, err)

			if di.Status == StatusWaitConfirm {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(dbScanTimeout):
		t.Fatal("Waiting to check for StatusWaitSend deposits timed out")
	}

	di, err := e.store.(*Store).getDepositInfo(dn.Deposit.ID())
	require.NoError(t, err)
	require.NotEmpty(t, di.UpdatedAt)
	require.Equal(t, DepositInfo{
		Seq:            1,
		CoinType:       scanner.CoinTypeBTC,
		UpdatedAt:      di.UpdatedAt,
		SkyAddress:     skyAddr,
		DepositAddress: btcAddr,
		DepositID:      dn.Deposit.ID(),
		Txid:           txid,
		SkySent:        100e6,
		DepositValue:   dn.Deposit.Value,
		BuyMethod:      config.BuyMethodDirect,
		Status:         StatusWaitConfirm,
		ConversionRate: testSkyBtcRate,
		Deposit:        dn.Deposit,
	}, di)

}

func TestExchangeQuitBeforeConfirm(t *testing.T) {
	e, shutdown, _ := runExchange(t)
	defer shutdown()

	skyAddr := testSkyAddr
	btcAddr := "foo-btc-addr"
	mustBindAddress(t, e.store, skyAddr, btcAddr)

	var value int64 = 1e8
	skySent, err := CalculateBtcSkyValue(value, testSkyBtcRate, testMaxDecimals)
	require.NoError(t, err)
	txid := e.Sender.(*Send).sender.(*dummySender).predictTxid(t, skyAddr, skySent)

	dn := scanner.DepositNote{
		Deposit: scanner.Deposit{
			CoinType: scanner.CoinTypeBTC,
			Address:  btcAddr,
			Value:    value,
			Height:   20,
			Tx:       "foo-tx",
			N:        2,
		},
		ErrC: make(chan error, 1),
	}
	mp := e.Receiver.(*Receive).multiplexer
	mp.GetScanner(scanner.CoinTypeBTC).(*dummyScanner).addDeposit(dn)

	// First loop calls saveIncomingDeposit
	// nil is written to ErrC after this method finishes
	err = <-dn.ErrC
	require.NoError(t, err)

	// Second loop calls processWaitSendDeposit
	// It sends the coins, then confirms them
	expectedDeposit := DepositInfo{
		Seq:            1,
		CoinType:       scanner.CoinTypeBTC,
		Status:         StatusWaitConfirm,
		SkyAddress:     skyAddr,
		DepositAddress: dn.Deposit.Address,
		DepositID:      dn.Deposit.ID(),
		Txid:           txid,
		SkySent:        100e6,
		BuyMethod:      config.BuyMethodDirect,
		DepositValue:   dn.Deposit.Value,
		ConversionRate: testSkyBtcRate,
		Deposit:        dn.Deposit,
	}

	// Periodically check the database until we observe the sent deposit
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range time.Tick(dbCheckWaitTime) {
			di, err := e.store.(*Store).getDepositInfo(dn.Deposit.ID())
			require.NoError(t, err)

			if di.Status != expectedDeposit.Status {
				continue
			}

			require.NotEmpty(t, di.UpdatedAt)

			ed := expectedDeposit
			ed.UpdatedAt = di.UpdatedAt

			require.Equal(t, ed, di)
			return
		}
	}()

	select {
	case <-done:
	case <-time.After(dbScanTimeout):
		t.Fatal("Waiting for sent deposit timed out")
	}

	e.Shutdown()

	di, err := e.store.(*Store).getDepositInfo(dn.Deposit.ID())
	require.NoError(t, err)

	require.NotEmpty(t, di.UpdatedAt)
	ed := expectedDeposit
	ed.UpdatedAt = di.UpdatedAt

	require.Equal(t, ed, di)
}

func TestExchangeSendZeroCoins(t *testing.T) {
	// Tests what happens when the scanner sends us an empty deposit value,
	// or the deposit value is so small that it is worth less than 1 SKY after
	// rate conversion.
	// The scanner should never do this, but we must handle it in case it happens
	e, shutdown, hook := runExchange(t)
	defer shutdown()

	skyAddr := testSkyAddr
	btcAddr := "foo-btc-addr"
	mustBindAddress(t, e.store, skyAddr, btcAddr)

	dn := scanner.DepositNote{
		Deposit: scanner.Deposit{
			CoinType: scanner.CoinTypeBTC,
			Address:  btcAddr,
			Value:    1, // The amount is so low that no SKY can be sent
			Height:   20,
			Tx:       "foo-tx",
			N:        2,
		},
		ErrC: make(chan error, 1),
	}
	mp := e.Receiver.(*Receive).multiplexer
	mp.GetScanner(scanner.CoinTypeBTC).(*dummyScanner).addDeposit(dn)

	// First loop calls saveIncomingDeposit
	// nil is written to ErrC after this method finishes
	err := <-dn.ErrC
	require.NoError(t, err)

	// Second loop calls processWaitSendDeposit
	// It sends the coins, then confirms them

	expectedDeposit := DepositInfo{
		Seq:            1,
		CoinType:       scanner.CoinTypeBTC,
		Status:         StatusDone,
		SkyAddress:     skyAddr,
		DepositAddress: dn.Deposit.Address,
		DepositID:      dn.Deposit.ID(),
		Txid:           "",
		SkySent:        0,
		ConversionRate: testSkyBtcRate,
		BuyMethod:      config.BuyMethodDirect,
		DepositValue:   dn.Deposit.Value,
		Deposit:        dn.Deposit,
		Error:          ErrEmptySendAmount.Error(),
	}

	// Periodically check the database until we observe the sent deposit
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range time.Tick(dbCheckWaitTime) {
			di, err := e.store.(*Store).getDepositInfo(dn.Deposit.ID())
			require.NoError(t, err)

			if di.Status != expectedDeposit.Status {
				continue
			}

			require.NotEmpty(t, di.UpdatedAt)

			ed := expectedDeposit
			ed.UpdatedAt = di.UpdatedAt

			require.Equal(t, ed, di)
			return
		}
	}()

	select {
	case <-done:
	case <-time.After(dbScanTimeout):
		t.Fatal("Waiting for sent deposit timed out")
	}

	e.Shutdown()

	di, err := e.store.(*Store).getDepositInfo(dn.Deposit.ID())
	require.NoError(t, err)

	require.NotEmpty(t, di.UpdatedAt)
	ed := expectedDeposit
	ed.UpdatedAt = di.UpdatedAt

	require.Equal(t, ed, di)

	loggedErrEmptySendAmount := false
	for _, e := range hook.AllEntries() {
		err, ok := e.Data["error"].(error)
		if ok && err == ErrEmptySendAmount {
			loggedErrEmptySendAmount = true
			break
		}
	}

	require.True(t, loggedErrEmptySendAmount)
}

func testExchangeRunProcessDepositBacklog(t *testing.T, dis []DepositInfo, configureSender func(*Exchange, DepositInfo)) {
	log, _ := testutil.NewLogger(t)
	e, run, shutdown := setupExchange(t, log)

	updatedDis := make([]DepositInfo, 0, len(dis))
	for _, di := range dis {
		err := di.ValidateForStatus()
		require.NoError(t, err)
		configureSender(e, di)

		updatedDi, err := e.store.(*Store).addDepositInfo(di)
		require.NoError(t, err)
		updatedDis = append(updatedDis, updatedDi)
	}

	dis = updatedDis

	filter := func(di DepositInfo) bool {
		return di.Status == StatusDone
	}

	// Make sure that there are no confirmed deposits yet
	confirmed, err := e.store.GetDepositInfoArray(filter)
	require.NoError(t, err)
	require.Len(t, confirmed, 0)

	// Run the exchange
	go run()
	defer shutdown()
	defer e.Shutdown()

	// Wait until we find 2 confirmed deposits
	done := make(chan struct{})
	complete := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-complete:
				return
			case <-time.After(dbCheckWaitTime):
				confirmed, err := e.store.GetDepositInfoArray(filter)
				require.NoError(t, err)
				if len(confirmed) == len(dis) {
					return
				}
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(dbScanTimeout):
		close(complete)
		t.Fatal("Waiting for confirmed deposits timed out")
	}

	confirmed, err = e.store.GetDepositInfoArray(filter)
	require.NoError(t, err)
	require.Len(t, confirmed, len(dis))

	// Verify the 2 confirmed deposits
	expectedDis := make([]DepositInfo, len(dis))
	for i, di := range dis {
		expectedDis[i] = di
		expectedDis[i].Status = StatusDone

		if expectedDis[i].SkySent == 0 {
			t.Logf("di.DepositValue=%d e.cfg.SkyBtcExchangeRate=%s", di.DepositValue, e.cfg.SkyBtcExchangeRate)
			amt, err := CalculateBtcSkyValue(di.DepositValue, e.cfg.SkyBtcExchangeRate, testMaxDecimals)
			require.NoError(t, err)
			expectedDis[i].SkySent = amt
		}

		require.NotEmpty(t, confirmed[i].UpdatedAt)
		expectedDis[i].UpdatedAt = confirmed[i].UpdatedAt

		require.Equal(t, expectedDis[i], confirmed[i])
	}
}

func TestExchangeProcessUnconfirmedTx(t *testing.T) {
	// Tests that StatusWaitConfirm deposits found in the db are processed
	// on exchange startup.

	var depositValue int64 = 1e8
	s := newDummySender()
	skySent, err := CalculateBtcSkyValue(depositValue, testSkyBtcRate, testMaxDecimals)
	require.NoError(t, err)
	txid1 := s.predictTxid(t, testSkyAddr, skySent)
	txid2 := s.predictTxid(t, testSkyAddr2, skySent)

	// Add StatusWaitConfirm deposits
	// They should all be confirmed after shutdown
	dis := []DepositInfo{
		{
			Seq:            1,
			Status:         StatusWaitConfirm,
			SkyAddress:     testSkyAddr,
			DepositAddress: "foo-btc-addr-1",
			DepositID:      "foo-tx-1:1",
			Txid:           txid1,
			SkySent:        skySent,
			ConversionRate: testSkyBtcRate,
			BuyMethod:      config.BuyMethodDirect,
			DepositValue:   depositValue,
			Deposit: scanner.Deposit{
				CoinType: scanner.CoinTypeBTC,
				Address:  "foo-btc-addr-1",
				Value:    depositValue,
				Height:   20,
				Tx:       "foo-tx-1",
				N:        1,
			},
		},
		{
			Seq:            2,
			Status:         StatusWaitConfirm,
			SkyAddress:     testSkyAddr2,
			DepositAddress: "foo-btc-addr-2",
			DepositID:      "foo-tx-2:2",
			Txid:           txid2,
			SkySent:        skySent,
			ConversionRate: testSkyBtcRate,
			BuyMethod:      config.BuyMethodDirect,
			DepositValue:   depositValue,
			Deposit: scanner.Deposit{
				CoinType: scanner.CoinTypeBTC,
				Address:  "foo-btc-addr-2",
				Value:    depositValue,
				Height:   20,
				Tx:       "foo-tx-2",
				N:        2,
			},
		},
	}

	testExchangeRunProcessDepositBacklog(t, dis, func(e *Exchange, di DepositInfo) {
		e.Sender.(*Send).sender.(*dummySender).setTxConfirmed(di.Txid)
	})
}

func TestExchangeProcessWaitSendDeposits(t *testing.T) {
	// Tests that StatusWaitSend deposits found in the db are processed
	// on exchange startup

	var depositValue int64 = 1e8
	s := newDummySender()
	skySent, err := CalculateBtcSkyValue(depositValue, testSkyBtcRate, testMaxDecimals)
	require.NoError(t, err)
	txid1 := s.predictTxid(t, testSkyAddr, skySent)
	txid2 := s.predictTxid(t, testSkyAddr2, skySent)

	// Add StatusWaitSend deposits
	// They should all be confirmed after shutdown
	dis := []DepositInfo{
		{
			Seq:            1,
			CoinType:       scanner.CoinTypeBTC,
			Status:         StatusWaitSend,
			SkyAddress:     testSkyAddr,
			DepositAddress: "foo-btc-addr-1",
			DepositID:      "foo-tx-1:1",
			Txid:           txid1,
			ConversionRate: testSkyBtcRate,
			DepositValue:   depositValue,
			BuyMethod:      config.BuyMethodDirect,
			Deposit: scanner.Deposit{
				CoinType: scanner.CoinTypeBTC,
				Address:  "foo-btc-addr-1",
				Value:    depositValue,
				Height:   20,
				Tx:       "foo-tx-1",
				N:        1,
			},
		},
		{
			Seq:            2,
			CoinType:       scanner.CoinTypeBTC,
			Status:         StatusWaitSend,
			SkyAddress:     testSkyAddr2,
			DepositAddress: "foo-btc-addr-2",
			DepositID:      "foo-tx-2:2",
			Txid:           txid2,
			ConversionRate: testSkyBtcRate,
			DepositValue:   depositValue,
			BuyMethod:      config.BuyMethodDirect,
			Deposit: scanner.Deposit{
				CoinType: scanner.CoinTypeBTC,
				Address:  "foo-btc-addr-2",
				Value:    depositValue,
				Height:   20,
				Tx:       "foo-tx-2",
				N:        2,
			},
		},
	}

	testExchangeRunProcessDepositBacklog(t, dis, func(e *Exchange, di DepositInfo) {
		boundAddr, err := e.store.BindAddress(di.SkyAddress, di.DepositAddress, di.CoinType, di.BuyMethod)
		require.NoError(t, err)
		require.Equal(t, di.SkyAddress, boundAddr.SkyAddress)
		require.Equal(t, di.DepositAddress, boundAddr.Address)
		require.Equal(t, di.CoinType, boundAddr.CoinType)
		require.Equal(t, di.BuyMethod, boundAddr.BuyMethod)

		skySent, err := CalculateBtcSkyValue(di.DepositValue, di.ConversionRate, testMaxDecimals)
		require.NoError(t, err)

		txid := e.Sender.(*Send).sender.(*dummySender).predictTxid(t, di.SkyAddress, skySent)
		e.Sender.(*Send).sender.(*dummySender).setTxConfirmed(txid)
	})
}

func TestExchangeProcessWaitDecideDeposits(t *testing.T) {
	// Tests that StatusWaitDecide deposits found in the db are processed
	// on exchange startup

	var depositValue int64 = 1e8
	s := newDummySender()
	skySent, err := CalculateBtcSkyValue(depositValue, testSkyBtcRate, testMaxDecimals)
	require.NoError(t, err)
	txid1 := s.predictTxid(t, testSkyAddr, skySent)
	txid2 := s.predictTxid(t, testSkyAddr2, skySent)

	// Add StatusWaitDecide deposits
	// They should all be confirmed after shutdown
	dis := []DepositInfo{
		{
			Seq:            1,
			CoinType:       scanner.CoinTypeBTC,
			Status:         StatusWaitDecide,
			SkyAddress:     testSkyAddr,
			DepositAddress: "foo-btc-addr-1",
			DepositID:      "foo-tx-1:1",
			Txid:           txid1,
			ConversionRate: testSkyBtcRate,
			DepositValue:   depositValue,
			BuyMethod:      config.BuyMethodDirect,
			Deposit: scanner.Deposit{
				CoinType: scanner.CoinTypeBTC,
				Address:  "foo-btc-addr-1",
				Value:    depositValue,
				Height:   20,
				Tx:       "foo-tx-1",
				N:        1,
			},
		},
		{
			Seq:            2,
			CoinType:       scanner.CoinTypeBTC,
			Status:         StatusWaitDecide,
			SkyAddress:     testSkyAddr2,
			DepositAddress: "foo-btc-addr-2",
			DepositID:      "foo-tx-2:2",
			Txid:           txid2,
			ConversionRate: testSkyBtcRate,
			DepositValue:   depositValue,
			BuyMethod:      config.BuyMethodDirect,
			Deposit: scanner.Deposit{
				CoinType: scanner.CoinTypeBTC,
				Address:  "foo-btc-addr-2",
				Value:    depositValue,
				Height:   20,
				Tx:       "foo-tx-2",
				N:        2,
			},
		},
	}

	testExchangeRunProcessDepositBacklog(t, dis, func(e *Exchange, di DepositInfo) {
		boundAddr, err := e.store.BindAddress(di.SkyAddress, di.DepositAddress, di.CoinType, di.BuyMethod)
		require.NoError(t, err)
		require.Equal(t, di.SkyAddress, boundAddr.SkyAddress)
		require.Equal(t, di.DepositAddress, boundAddr.Address)
		require.Equal(t, di.CoinType, boundAddr.CoinType)
		require.Equal(t, di.BuyMethod, boundAddr.BuyMethod)

		skySent, err := CalculateBtcSkyValue(di.DepositValue, di.ConversionRate, testMaxDecimals)
		require.NoError(t, err)

		txid := e.Sender.(*Send).sender.(*dummySender).predictTxid(t, di.SkyAddress, skySent)
		e.Sender.(*Send).sender.(*dummySender).setTxConfirmed(txid)
	})
}

func TestExchangeSaveIncomingDepositCreateDepositFailed(t *testing.T) {
	// Tests that we log a message and continue if saveIncomingDeposit fails
	e, shutdown, hook := runExchangeMockStore(t)
	defer shutdown()
	didShutdown := false
	defer func() {
		if !didShutdown {
			e.Shutdown()
		}
	}()

	btcAddr := "foo-btc-addr"

	dn := scanner.DepositNote{
		Deposit: scanner.Deposit{
			CoinType: scanner.CoinTypeBTC,
			Address:  btcAddr,
			Value:    1e8,
			Height:   20,
			Tx:       "foo-tx",
			N:        2,
		},
		ErrC: make(chan error, 1),
	}
	mp := e.Receiver.(*Receive).multiplexer
	mp.GetScanner(scanner.CoinTypeBTC).(*dummyScanner).addDeposit(dn)

	// Configure database mocks

	// GetDepositInfoArray is called twice on startup
	e.store.(*MockStore).On("GetDepositInfoArray", mock.MatchedBy(func(filt DepositFilter) bool {
		return true
	})).Return(nil, nil).Times(3)

	// Return error on GetOrCreateDepositInfo
	createDepositErr := errors.New("GetOrCreateDepositInfo failed")
	e.store.(*MockStore).On("GetOrCreateDepositInfo", dn.Deposit, testSkyBtcRate).Return(DepositInfo{}, createDepositErr)

	// First loop calls saveIncomingDeposit
	// err is written to ErrC after this method finishes
	err := <-dn.ErrC
	require.Error(t, err)
	require.Equal(t, createDepositErr, err)

	// Check that we logged the failed save, so that we can recover it later
	logEntry := hook.LastEntry()
	require.Equal(t, logEntry.Message, "saveIncomingDeposit failed. This deposit will not be reprocessed until teller is restarted.")
	loggedDeposit := logEntry.Data["deposit"].(scanner.Deposit)
	require.Equal(t, dn.Deposit, loggedDeposit)
}

func TestExchangeProcessWaitSendDepositFailed(t *testing.T) {
	// Tests that we log a message and continue if processWaitSendDeposit fails
	e, shutdown, hook := runExchangeMockStore(t)
	defer shutdown()
	didShutdown := false
	defer func() {
		if !didShutdown {
			e.Shutdown()
		}
	}()

	skyAddr := testSkyAddr
	btcAddr := "foo-btc-addr"

	dn := scanner.DepositNote{
		Deposit: scanner.Deposit{
			CoinType: scanner.CoinTypeBTC,
			Address:  btcAddr,
			Value:    1e8,
			Height:   20,
			Tx:       "foo-tx",
			N:        2,
		},
		ErrC: make(chan error, 1),
	}
	mp := e.Receiver.(*Receive).multiplexer
	mp.GetScanner(scanner.CoinTypeBTC).(*dummyScanner).addDeposit(dn)

	// Configure database mocks

	// GetDepositInfoArray is called twice on startup
	e.store.(*MockStore).On("GetDepositInfoArray", mock.MatchedBy(func(filt DepositFilter) bool {
		return true
	})).Return(nil, nil).Times(3)

	// GetBindAddress returns a bound address
	e.store.(*MockStore).On("GetBindAddress", btcAddr).Return(skyAddr, nil)

	// GetOrCreateDepositInfo returns a valid DepositInfo
	di := DepositInfo{
		Seq:            1,
		CoinType:       scanner.CoinTypeBTC,
		Status:         StatusWaitSend,
		SkyAddress:     skyAddr,
		DepositAddress: btcAddr,
		DepositID:      dn.Deposit.ID(),
		BuyMethod:      config.BuyMethodDirect,
		ConversionRate: testSkyBtcRate,
		Deposit:        dn.Deposit,
	}
	e.store.(*MockStore).On("GetOrCreateDepositInfo", dn.Deposit, testSkyBtcRate).Return(di, nil)

	// UpdateDepositInfo fails
	updateDepositInfoErr := errors.New("UpdateDepositInfo error")
	e.store.(*MockStore).On("UpdateDepositInfo", di.DepositID, mock.MatchedBy(func(f func(DepositInfo) DepositInfo) bool {
		return true
	})).Return(DepositInfo{}, updateDepositInfoErr)

	// First loop calls saveIncomingDeposit
	// nil is written to ErrC after this method finishes
	err := <-dn.ErrC
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range time.Tick(dbCheckWaitTime) {
			for _, e := range hook.AllEntries() {
				if strings.Contains(e.Message, "updateStatus failed") {
					return
				}
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(dbScanTimeout):
		t.Fatal("Waiting for deposit send failure timed out")
	}

	didShutdown = true
	e.Shutdown()

	foundMsg := false
	for _, e := range hook.AllEntries() {
		if !strings.Contains(e.Message, "updateStatus failed") {
			continue
		}
		foundMsg = true
		require.Equal(t, "updateStatus failed. This deposit will not be reprocessed until teller is restarted.", e.Message)
		loggedDepositInfo, ok := e.Data["depositInfo"].(DepositInfo)
		require.True(t, ok)
		require.Equal(t, di, loggedDepositInfo)
	}

	require.True(t, foundMsg)
}

func TestExchangeProcessWaitSendNoSkyAddrBound(t *testing.T) {
	// Tests that we log a message and continue if processWaitSendDeposit fails
	e, shutdown, hook := runExchange(t)
	defer shutdown()
	defer e.Shutdown()

	btcAddr := "foo-btc-addr"

	dn := scanner.DepositNote{
		Deposit: scanner.Deposit{
			CoinType: scanner.CoinTypeBTC,
			Address:  btcAddr,
			Value:    1e8,
			Height:   20,
			Tx:       "foo-tx",
			N:        2,
		},
		ErrC: make(chan error, 1),
	}
	mp := e.Receiver.(*Receive).multiplexer
	mp.GetScanner(scanner.CoinTypeBTC).(*dummyScanner).addDeposit(dn)

	// First loop calls saveIncomingDeposit
	// nil is written to ErrC after this method finishes
	err := <-dn.ErrC
	require.Error(t, err)
	require.Equal(t, err, ErrNoBoundAddress)

	// Check that we logged the failed save, so that we can recover it later
	logEntry := hook.LastEntry()
	require.Equal(t, logEntry.Message, "saveIncomingDeposit failed. This deposit will not be reprocessed until teller is restarted.")
	loggedDeposit := logEntry.Data["deposit"].(scanner.Deposit)
	require.Equal(t, dn.Deposit, loggedDeposit)
}

func TestExchangeBindAddress(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)
	store, err := NewStore(log, db)
	require.NoError(t, err)
	dummyScanner := newDummyScanner()
	multiplexer := scanner.NewMultiplexer(log)
	err = multiplexer.AddScanner(dummyScanner, scanner.CoinTypeBTC)
	require.NoError(t, err)

	s, err := NewDirectExchange(log, defaultCfg, store, multiplexer, nil)
	require.NoError(t, err)

	require.Len(t, dummyScanner.addrs, 0)

	boundAddr, err := s.BindAddress("a", "b", scanner.CoinTypeBTC)
	require.NoError(t, err)
	require.Equal(t, "a", boundAddr.SkyAddress)
	require.Equal(t, "b", boundAddr.Address)

	// Should be added to dummyScanner
	require.Len(t, dummyScanner.addrs, 1)
	require.Equal(t, "b", dummyScanner.addrs[0])

	// Should be in the store
	skyAddr, err := s.store.GetBindAddress("b", scanner.CoinTypeBTC)
	require.NoError(t, err)
	require.Equal(t, &BoundAddress{
		SkyAddress: "a",
		Address:    "b",
		CoinType:   scanner.CoinTypeBTC,
		BuyMethod:  config.BuyMethodDirect,
	}, skyAddr)
}

func TestExchangeCreateTransaction(t *testing.T) {
	cfg := defaultCfg
	cfg.SkyBtcExchangeRate = "111"

	log, _ := testutil.NewLogger(t)
	s, err := NewDirectExchange(log, cfg, nil, nil, newDummySender())
	require.NoError(t, err)

	// Create transaction with no SkyAddress
	di := DepositInfo{
		CoinType:       scanner.CoinTypeBTC,
		SkyAddress:     "",
		DepositValue:   1e8,
		ConversionRate: "100",
	}

	_, err = s.Sender.(*Send).createTransaction(di)
	require.Equal(t, ErrNoBoundAddress, err)

	// Create transaction with no coins sent, due to a very low DepositValue
	di = DepositInfo{
		CoinType:       scanner.CoinTypeBTC,
		SkyAddress:     "2GgFvqoyk9RjwVzj8tqfcXVXB4orBwoc9qv",
		DepositValue:   1,
		ConversionRate: "100",
	}
	_, err = s.Sender.(*Send).createTransaction(di)
	require.Equal(t, ErrEmptySendAmount, err)

	// Create valid transaction
	di = DepositInfo{
		CoinType:       scanner.CoinTypeBTC,
		SkyAddress:     "2GgFvqoyk9RjwVzj8tqfcXVXB4orBwoc9qv",
		DepositValue:   1e8,
		ConversionRate: "100",
	}
	// Make sure DepositInfo.ConversionRate != s.cfg.SkyBtcExchangeRate, to confirm
	// that the DepositInfo's ConversionRate is used instead of cfg.SkyBtcExchangeRate
	require.NotEqual(t, s.cfg.SkyBtcExchangeRate, di.ConversionRate)

	tx, err := s.Sender.(*Send).createTransaction(di)
	require.NoError(t, err)
	// Should have one output for destination and one for change
	require.Len(t, tx.Out, 2)

	// Check that the coins sent are based upon the DepositValue.ConversionRate
	var txOut *coin.TransactionOutput
	for _, o := range tx.Out {
		if o.Address.String() == di.SkyAddress {
			txOut = &o
			break
		}
	}
	require.NotNil(t, txOut)
	require.Equal(t, uint64(100e6), txOut.Coins)
}

func TestExchangeGetDepositStatuses(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)
	store, err := NewStore(log, db)
	require.NoError(t, err)
	dummyScanner := newDummyScanner()
	dummyScannerEth := newDummyScanner()
	multiplexer := scanner.NewMultiplexer(log)
	err = multiplexer.AddScanner(dummyScanner, scanner.CoinTypeBTC)
	require.NoError(t, err)
	err = multiplexer.AddScanner(dummyScannerEth, scanner.CoinTypeETH)
	require.NoError(t, err)

	s, err := NewDirectExchange(log, defaultCfg, store, multiplexer, nil)
	require.NoError(t, err)

	require.Len(t, dummyScanner.addrs, 0)

	boundAddr, err := s.BindAddress("a", "b", scanner.CoinTypeBTC)
	require.NoError(t, err)
	require.Equal(t, "a", boundAddr.SkyAddress)
	require.Equal(t, "b", boundAddr.Address)

	boundAddr, err = s.BindAddress("a", "e", scanner.CoinTypeETH)
	require.NoError(t, err)
	require.Equal(t, "a", boundAddr.SkyAddress)
	require.Equal(t, "e", boundAddr.Address)

	// Should be added to dummyScanner
	require.Len(t, dummyScanner.addrs, 1)
	require.Equal(t, "b", dummyScanner.addrs[0])

	require.Len(t, dummyScannerEth.addrs, 1)
	require.Equal(t, "e", dummyScannerEth.addrs[0])

	// Should be in the store
	depositInfos, err := s.store.GetDepositInfoOfSkyAddress("a")
	require.NoError(t, err)
	require.Equal(t, 2, len(depositInfos))
	depositInfo := depositInfos[0]
	require.Equal(t, uint64(0), depositInfo.Seq)
	require.Equal(t, scanner.CoinTypeBTC, depositInfo.CoinType)
	require.Equal(t, StatusWaitDeposit, depositInfo.Status)
	require.NotEmpty(t, depositInfo.UpdatedAt)
	depositInfo = depositInfos[1]
	require.Equal(t, uint64(1), depositInfo.Seq)
	require.Equal(t, scanner.CoinTypeETH, depositInfo.CoinType)
	require.Equal(t, StatusWaitDeposit, depositInfo.Status)
	require.NotEmpty(t, depositInfo.UpdatedAt)
}

func TestExchangeGetDepositStatusDetail(t *testing.T) {
	// TODO
}

func TestExchangeGetBindNum(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)
	store, err := NewStore(log, db)
	require.NoError(t, err)

	bscr := newDummyScanner()
	multiplexer := scanner.NewMultiplexer(log)
	err = multiplexer.AddScanner(bscr, scanner.CoinTypeBTC)
	require.NoError(t, err)

	s, err := NewDirectExchange(log, defaultCfg, store, multiplexer, nil)
	require.NoError(t, err)

	num, err := s.GetBindNum("a")
	require.Equal(t, num, 0)
	require.NoError(t, err)

	mustBindAddress(t, store, "a", "b")

	num, err = s.GetBindNum("a")
	require.NoError(t, err)
	require.Equal(t, num, 1)
}
