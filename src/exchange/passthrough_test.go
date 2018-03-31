package exchange

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	logrus_test "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/exchange-api/exchange/c2cx"

	"github.com/skycoin/teller/src/config"
	"github.com/skycoin/teller/src/scanner"
	"github.com/skycoin/teller/src/util/testutil"
)

var (
	defaultPassthroughCfg = config.SkyExchanger{
		SkyBtcExchangeRate:      testSkyBtcRate,
		SkyEthExchangeRate:      testSkyEthRate,
		SkySkyExchangeRate:      testSkySkyRate,
		MaxDecimals:             3,
		TxConfirmationCheckWait: time.Second,
		Wallet:                  testWalletFile,
		SendEnabled:             true,
		BuyMethod:               config.BuyMethodPassthrough,
		C2CX: config.C2CX{
			Key:                "c2cx-key",
			Secret:             "c2cx-secret",
			RequestFailureWait: time.Millisecond * 30,
			RatelimitWait:      time.Millisecond * 60,
			CheckOrderWait:     time.Millisecond * 10,
			BtcMinimumVolume:   decimal.New(1, -3),
		},
	}
)

type mockReceiver struct {
	deposits chan DepositInfo
}

func newMockReceiver() *mockReceiver {
	return &mockReceiver{
		deposits: make(chan DepositInfo, 100),
	}
}

func (m *mockReceiver) Deposits() <-chan DepositInfo {
	return m.deposits
}

func (m *mockReceiver) BindAddress(a, b, c, d string) (*BoundAddress, error) {
	return nil, errors.New("mockReceiver.BindAddress not implemented")
}

func createDepositStatusWaitDecide(t *testing.T, p *Passthrough, skyAddr string, n uint32) DepositInfo {
	btcAddr := testutil.RandString(t, 16)
	_, err := p.store.BindAddress(skyAddr, btcAddr, config.CoinTypeBTC, config.BuyMethodPassthrough)
	require.NoError(t, err)

	depositInfo, err := p.store.GetOrCreateDepositInfo(scanner.Deposit{
		CoinType:  config.CoinTypeBTC,
		Address:   btcAddr,
		Value:     1000000,
		Height:    400000,
		Tx:        "btc-tx-id",
		N:         n,
		Processed: true,
	}, defaultPassthroughCfg.SkyBtcExchangeRate)
	require.NoError(t, err)

	return depositInfo
}

func createDepositStatusWaitPassthrough(t *testing.T, p *Passthrough, skyAddr string, n uint32) DepositInfo {
	depositInfo := createDepositStatusWaitDecide(t, p, skyAddr, n)

	depositInfo, err := p.store.UpdateDepositInfo(depositInfo.DepositID, func(di DepositInfo) DepositInfo {
		di.Status = StatusWaitPassthrough
		di.Passthrough.ExchangeName = PassthroughExchangeC2CX
		di.Passthrough.RequestedAmount = calculateRequestedAmount(di.DepositValue).String()
		di.Passthrough.Order.CustomerID = di.DepositID
		return di
	})
	require.NoError(t, err)

	return depositInfo
}

func createDepositStatusWaitPassthroughOrderComplete(t *testing.T, p *Passthrough, skyAddr string, n uint32, orderID c2cx.OrderID) DepositInfo {
	depositInfo := createDepositStatusWaitPassthrough(t, p, skyAddr, n)

	depositInfo, err := p.store.UpdateDepositInfo(depositInfo.DepositID, func(di DepositInfo) DepositInfo {
		di.Status = StatusWaitPassthroughOrderComplete
		di.Passthrough.Order.OrderID = fmt.Sprint(orderID)
		return di
	})
	require.NoError(t, err)

	return depositInfo
}

func setupPassthrough(t *testing.T) (*Passthrough, func(), *MockC2CXClient, *logrus_test.Hook) {
	db, shutdown := testutil.PrepareDB(t)

	log, hook := testutil.NewLogger(t)
	store, err := NewStore(log, db)
	require.NoError(t, err)

	receiver := newMockReceiver()

	p, err := NewPassthrough(log, defaultPassthroughCfg, store, receiver)
	require.NoError(t, err)
	require.False(t, p.exchangeClient.(*c2cx.Client).Debug, "c2cx client debug should be off")

	mockClient := &MockC2CXClient{}
	p.exchangeClient = mockClient

	return p, shutdown, mockClient, hook
}

func TestPassthroughStartupFailure(t *testing.T) {
	// Tests what happens when Run() initialization fails
	// If there are any StatusWaitPassthrough deposits, fixUnrecordedOrders()
	// will try to match them by CustomerID to orders in our orderbook on the exchange.
	// When it calls GetOrderByStatus, fail, and check that Run() exits with this error.
	p, shutdown, mockClient, _ := setupPassthrough(t)
	defer shutdown()

	getOrderByStatusErr := errors.New("GetOrderByStatus failure")
	mockClient.On("GetOrderByStatus", c2cx.BtcSky, c2cx.StatusAll).Return(nil, getOrderByStatusErr)

	createDepositStatusWaitPassthrough(t, p, testSkyAddr, 0)

	err := p.Run()
	require.Error(t, err)
	require.Equal(t, getOrderByStatusErr, err)
}

func TestPassthroughFixUnrecordedOrders(t *testing.T) {
	p, shutdown, mockClient, _ := setupPassthrough(t)
	defer shutdown()

	di := createDepositStatusWaitPassthrough(t, p, testSkyAddr, 0)
	require.NotEmpty(t, di.Passthrough.Order.CustomerID)

	orderID := 1234
	otherCustomerID := "abcd"

	orders := []c2cx.Order{
		{
			OrderID:    c2cx.OrderID(orderID),
			CustomerID: &otherCustomerID,
		},
		{
			OrderID:    c2cx.OrderID(orderID + 1),
			CustomerID: nil,
		},
		{
			OrderID:    c2cx.OrderID(orderID + 2),
			CustomerID: &di.Passthrough.Order.CustomerID,
		},
	}

	mockClient.On("GetOrderByStatus", c2cx.BtcSky, c2cx.StatusAll).Return(orders, nil)

	updates, err := p.fixUnrecordedOrders()
	require.NoError(t, err)

	require.Len(t, updates, 1)

	updatedDi := updates[0]
	require.Equal(t, di.DepositID, updatedDi.DepositID)
	require.Equal(t, StatusWaitPassthroughOrderComplete, updatedDi.Status)
	require.Equal(t, fmt.Sprint(orderID+2), updatedDi.Passthrough.Order.OrderID)
}

func TestPassthroughLoadExistingDeposits(t *testing.T) {
	// Tests that existing StatusWaitPassthrough and StatusWaitPassthroughOrderComplete
	// deposits are loaded and processed.
	p, shutdown, mockClient, _ := setupPassthrough(t)
	defer shutdown()

	// Create a StatusWaitPassthrough and StatusWaitPassthroughOrderComplete
	orderIDWaitPassthrough := c2cx.OrderID(1234)
	orderIDWaitPassthroughOrderComplete := c2cx.OrderID(1235)

	diWaitPassthrough := createDepositStatusWaitPassthrough(t, p, testSkyAddr, 0)
	diWaitPassthrough, err := p.store.UpdateDepositInfo(diWaitPassthrough.DepositID, func(di DepositInfo) DepositInfo {
		di.DepositValue = 7e5
		di.Passthrough.RequestedAmount = "0.007"
		return di
	})
	require.NoError(t, err)

	diWaitPassthroughOrderComplete := createDepositStatusWaitPassthroughOrderComplete(t, p, testSkyAddr2, 1, orderIDWaitPassthroughOrderComplete)

	orderWaitPassthrough := &c2cx.Order{
		OrderID:         orderIDWaitPassthrough,
		CustomerID:      &diWaitPassthrough.Passthrough.Order.CustomerID,
		Status:          c2cx.StatusCompleted,
		CompletedAmount: decimal.New(123, -2),
		AvgPrice:        decimal.New(182, -5),
	}

	orderWaitPassthroughOrderComplete := &c2cx.Order{
		OrderID:         orderIDWaitPassthroughOrderComplete,
		CustomerID:      &diWaitPassthroughOrderComplete.Passthrough.Order.CustomerID,
		Status:          c2cx.StatusCompleted,
		CompletedAmount: decimal.New(345, -2),
		AvgPrice:        decimal.New(201, -5),
	}

	// GetOrderByStatus should return no matching orders
	mockClient.On("GetOrderByStatus", c2cx.BtcSky, c2cx.StatusAll).Return(nil, nil).Once()

	mockClient.On("GetBalanceSummary").Return(&c2cx.BalanceSummary{
		Balance: c2cx.Balances{
			Btc: decimal.New(5, 0),
		},
	}, nil).Once()

	requestedAmount := calculateRequestedAmount(diWaitPassthrough.DepositValue)
	mockClient.On("MarketBuy", c2cx.BtcSky, mock.MatchedBy(func(v decimal.Decimal) bool {
		return v.Equal(requestedAmount)
	}), &diWaitPassthrough.Passthrough.Order.CustomerID).Return(orderIDWaitPassthrough, nil).Once()

	// GetOrderInfo returns twice, successful deposits
	mockClient.On("GetOrderInfo", c2cx.BtcSky, orderIDWaitPassthrough).Return(orderWaitPassthrough, nil)
	mockClient.On("GetOrderInfo", c2cx.BtcSky, orderIDWaitPassthroughOrderComplete).Return(orderWaitPassthroughOrderComplete, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := p.Run()
		require.NoError(t, err)
	}()

	// Deposits should be found on .Deposits() in the correct order and with StatusWaitSend
	timeout := time.Second * 6
	select {
	case <-time.After(timeout):
		t.Fatal("Timed out waiting for the first deposit to process")
	case deposit := <-p.Deposits():
		// The first deposit should be the StatusWaitPassthroughOrderComplete one
		require.Equal(t, diWaitPassthroughOrderComplete.DepositID, deposit.DepositID)
		require.Equal(t, StatusWaitSend, deposit.Status)
		require.Equal(t, c2cx.StatusCompleted.String(), deposit.Passthrough.Order.Status)
		require.Equal(t, fmt.Sprint(orderIDWaitPassthroughOrderComplete), deposit.Passthrough.Order.OrderID)
		require.Equal(t, diWaitPassthroughOrderComplete.DepositID, deposit.Passthrough.Order.CustomerID)
		require.Equal(t, "0.00201", deposit.Passthrough.Order.Price)
		require.Equal(t, "3.45", deposit.Passthrough.Order.CompletedAmount)
		require.Equal(t, uint64(345e4), deposit.Passthrough.SkyBought)
		require.Equal(t, int64(693450), deposit.Passthrough.DepositValueSpent)
		require.True(t, deposit.Passthrough.Order.Final)
		require.NotEmpty(t, deposit.Passthrough.Order.Original)
		require.Empty(t, deposit.Error)
	}

	select {
	case <-time.After(timeout):
		t.Fatal("Timed out waiting for the second deposit to process")
	case deposit := <-p.Deposits():
		// The second deposit should be the StatusWaitPassthrough one
		require.Equal(t, diWaitPassthrough.DepositID, deposit.DepositID)
		require.Equal(t, StatusWaitSend, deposit.Status)
		require.Equal(t, c2cx.StatusCompleted.String(), deposit.Passthrough.Order.Status)
		require.Equal(t, fmt.Sprint(orderIDWaitPassthrough), deposit.Passthrough.Order.OrderID)
		require.Equal(t, diWaitPassthrough.DepositID, deposit.Passthrough.Order.CustomerID)
		require.Equal(t, "0.00182", deposit.Passthrough.Order.Price)
		require.Equal(t, "1.23", deposit.Passthrough.Order.CompletedAmount)
		require.Equal(t, uint64(123e4), deposit.Passthrough.SkyBought)
		require.Equal(t, int64(223860), deposit.Passthrough.DepositValueSpent)
		require.True(t, deposit.Passthrough.Order.Final)
		require.NotEmpty(t, deposit.Passthrough.Order.Original)
		require.Empty(t, deposit.Error)
	}

	select {
	case <-p.Deposits():
		t.Fatal("Did not expect a deposit")
	default:
	}

	p.Shutdown()

	wg.Wait()
}

func TestPassthroughOrderNotCompleted(t *testing.T) {
	// Tests that it loops until an order status is completed
	p, shutdown, mockClient, _ := setupPassthrough(t)
	defer shutdown()

	orderID := c2cx.OrderID(1234)

	di := createDepositStatusWaitPassthroughOrderComplete(t, p, testSkyAddr, 0, orderID)

	orderIncomplete := &c2cx.Order{
		OrderID:    orderID,
		CustomerID: &di.Passthrough.Order.CustomerID,
		Status:     c2cx.StatusPartial,
	}

	orderComplete := &c2cx.Order{
		OrderID:         orderID,
		CustomerID:      &di.Passthrough.Order.CustomerID,
		Status:          c2cx.StatusCompleted,
		CompletedAmount: decimal.New(123, -2),
		AvgPrice:        decimal.New(182, -5),
	}

	mockClient.On("GetOrderByStatus", c2cx.BtcSky, c2cx.StatusAll).Return(nil, nil)

	// Return an order with each of the possible "wait" statuses
	incompleteStatuses := []c2cx.OrderStatus{
		c2cx.StatusPartial,
		c2cx.StatusPending,
		c2cx.StatusActive,
		c2cx.StatusSuspended,
		c2cx.StatusTriggerPending,
		c2cx.StatusStopLossPending,
	}
	for _, s := range incompleteStatuses {
		order := orderIncomplete
		order.Status = s
		mockClient.On("GetOrderInfo", c2cx.BtcSky, orderID).Return(order, nil).Once()
	}

	// Return an order with a "complete" status
	mockClient.On("GetOrderInfo", c2cx.BtcSky, orderID).Return(orderComplete, nil).Once()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := p.Run()
		require.NoError(t, err)
	}()

	// Deposits should be found on .Deposits() in the correct order and with StatusWaitSend
	timeout := time.Second * 6
	select {
	case <-time.After(timeout):
		t.Fatal("Timed out waiting for the deposit to process")
	case deposit := <-p.Deposits():
		// The first deposit should be the StatusWaitPassthroughOrderComplete one
		require.Equal(t, di.DepositID, deposit.DepositID)
		require.Equal(t, StatusWaitSend, deposit.Status)
		require.Equal(t, c2cx.StatusCompleted.String(), deposit.Passthrough.Order.Status)
		require.Equal(t, fmt.Sprint(orderID), deposit.Passthrough.Order.OrderID)
		require.Equal(t, di.DepositID, deposit.Passthrough.Order.CustomerID)
		require.Equal(t, "0.00182", deposit.Passthrough.Order.Price)
		require.Equal(t, "1.23", deposit.Passthrough.Order.CompletedAmount)
		require.Equal(t, uint64(123e4), deposit.Passthrough.SkyBought)
		require.Equal(t, int64(223860), deposit.Passthrough.DepositValueSpent)
		require.True(t, deposit.Passthrough.Order.Final)
		require.NotEmpty(t, deposit.Passthrough.Order.Original)
		require.Empty(t, deposit.Error)

		// Check serialized original order

		deposit, err := p.store.(*Store).getDepositInfo(deposit.DepositID)
		require.NoError(t, err)

		var order c2cx.Order
		err = json.Unmarshal([]byte(deposit.Passthrough.Order.Original), &order)
		require.NoError(t, err)
		compareOrder(t, orderComplete, &order)
	}

	select {
	case <-p.Deposits():
		t.Fatal("Did not expect a deposit")
	default:
	}

	p.Shutdown()

	wg.Wait()
}

func TestPassthroughOrderFatalStatus(t *testing.T) {
	// Tests that if an order has a fatal status, the deposit skips to StatusDone
	p, shutdown, mockClient, hook := setupPassthrough(t)
	defer shutdown()

	orderID := c2cx.OrderID(1234)

	di := createDepositStatusWaitPassthroughOrderComplete(t, p, testSkyAddr, 0, orderID)

	orderFatal := &c2cx.Order{
		OrderID:         orderID,
		CustomerID:      &di.Passthrough.Order.CustomerID,
		Status:          c2cx.StatusExpired,
		CompletedAmount: decimal.New(123, -2),
		AvgPrice:        decimal.New(182, -5),
	}

	mockClient.On("GetOrderByStatus", c2cx.BtcSky, c2cx.StatusAll).Return(nil, nil)

	// Return an order with a "fatal" status
	mockClient.On("GetOrderInfo", c2cx.BtcSky, orderID).Return(orderFatal, nil).Once()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := p.Run()
		require.NoError(t, err)
	}()

	// Periodically check the database until we observe the sent deposit
	done := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(dbCheckWaitTime):
				deposit, err := p.store.(*Store).getDepositInfo(di.DepositID)
				require.NoError(t, err)

				if deposit.Status == StatusDone {
					return
				}
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(dbScanTimeout):
		close(stop)
		<-done
		t.Fatal("Waiting for sent deposit timed out")
	}

	deposit, err := p.store.(*Store).getDepositInfo(di.DepositID)
	require.NoError(t, err)

	require.Equal(t, di.DepositID, deposit.DepositID)
	require.Equal(t, StatusDone, deposit.Status)
	require.Equal(t, c2cx.StatusExpired.String(), deposit.Passthrough.Order.Status)
	require.Equal(t, fmt.Sprint(orderID), deposit.Passthrough.Order.OrderID)
	require.Equal(t, di.DepositID, deposit.Passthrough.Order.CustomerID)
	require.Equal(t, "", deposit.Passthrough.Order.Price)
	require.Equal(t, "", deposit.Passthrough.Order.CompletedAmount)
	require.Equal(t, uint64(0), deposit.Passthrough.SkyBought)
	require.Equal(t, int64(0), deposit.Passthrough.DepositValueSpent)
	require.True(t, deposit.Passthrough.Order.Final)
	require.Equal(t, ErrFatalOrderStatus.Error(), deposit.Error)
	require.NotEmpty(t, deposit.Passthrough.Order.Original)

	var order c2cx.Order
	err = json.Unmarshal([]byte(deposit.Passthrough.Order.Original), &order)
	require.NoError(t, err)
	compareOrder(t, orderFatal, &order)

	// Look for a log entry stating "will never be reprocessed"
	done = make(chan struct{})
	stop = make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(dbCheckWaitTime):
				for _, e := range hook.AllEntries() {
					if strings.Contains(e.Message, "will never be reprocessed") {
						return
					}
				}
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(dbScanTimeout):
		close(stop)
		<-done
		t.Fatal("Waiting for log entry timed out")
	}

	p.Shutdown()

	wg.Wait()
}

func TestPassthroughOrderVolumeTooLow(t *testing.T) {
	// Tests that if a deposit amount is too low, the deposit skips to StatusDone
	p, shutdown, mockClient, hook := setupPassthrough(t)
	defer shutdown()

	orderID := c2cx.OrderID(1234)

	di := createDepositStatusWaitPassthroughOrderComplete(t, p, testSkyAddr, 0, orderID)

	mockClient.On("GetOrderByStatus", c2cx.BtcSky, c2cx.StatusAll).Return(nil, nil)

	// Return an order with "limit value" error
	getOrderInfoErr := c2cx.NewAPIError("getorderinfo", http.StatusBadRequest, "limit value: 0.186")
	mockClient.On("GetOrderInfo", c2cx.BtcSky, orderID).Return(nil, getOrderInfoErr).Once()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := p.Run()
		require.NoError(t, err)
	}()

	// Periodically check the database until we observe the sent deposit
	done := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(dbCheckWaitTime):
				deposit, err := p.store.(*Store).getDepositInfo(di.DepositID)
				require.NoError(t, err)

				if deposit.Status == StatusDone {
					return
				}
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(dbScanTimeout):
		close(stop)
		<-done
		t.Fatal("Waiting for sent deposit timed out")
	}

	deposit, err := p.store.(*Store).getDepositInfo(di.DepositID)
	require.NoError(t, err)

	require.Equal(t, di.DepositID, deposit.DepositID)
	require.Equal(t, StatusDone, deposit.Status)
	require.Empty(t, deposit.Passthrough.Order.Status)
	require.Equal(t, fmt.Sprint(orderID), deposit.Passthrough.Order.OrderID)
	require.Equal(t, di.DepositID, deposit.Passthrough.Order.CustomerID)
	require.Equal(t, "", deposit.Passthrough.Order.Price)
	require.Equal(t, "", deposit.Passthrough.Order.CompletedAmount)
	require.Equal(t, uint64(0), deposit.Passthrough.SkyBought)
	require.Equal(t, int64(0), deposit.Passthrough.DepositValueSpent)
	require.False(t, deposit.Passthrough.Order.Final)
	require.Equal(t, getOrderInfoErr.Error(), deposit.Error)

	// Look for a log entry stating "will never be reprocessed"
	done = make(chan struct{})
	stop = make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(dbCheckWaitTime):
				for _, e := range hook.AllEntries() {
					if strings.Contains(e.Message, "will never be reprocessed") {
						return
					}
				}
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(dbScanTimeout):
		close(stop)
		<-done
		t.Fatal("Waiting for log entry timed out")
	}

	p.Shutdown()

	wg.Wait()
}

func testPassthroughRetryError(t *testing.T, getOrderInfoErr error) {
	p, shutdown, mockClient, _ := setupPassthrough(t)
	defer shutdown()

	orderID := c2cx.OrderID(1234)

	di := createDepositStatusWaitPassthroughOrderComplete(t, p, testSkyAddr, 0, orderID)

	orderComplete := &c2cx.Order{
		OrderID:         orderID,
		CustomerID:      &di.Passthrough.Order.CustomerID,
		Status:          c2cx.StatusCompleted,
		CompletedAmount: decimal.New(123, -2),
		AvgPrice:        decimal.New(182, -5),
	}

	mockClient.On("GetOrderByStatus", c2cx.BtcSky, c2cx.StatusAll).Return(nil, nil)

	mockClient.On("GetOrderInfo", c2cx.BtcSky, orderID).Return(nil, getOrderInfoErr).Once()

	// Return an order with a "complete" status
	mockClient.On("GetOrderInfo", c2cx.BtcSky, orderID).Return(orderComplete, nil).Once()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := p.Run()
		require.NoError(t, err)
	}()

	// Deposits should be found on .Deposits() in the correct order and with StatusWaitSend
	timeout := time.Second * 6
	select {
	case <-time.After(timeout):
		t.Fatal("Timed out waiting for the deposit to process")
	case deposit := <-p.Deposits():
		// The first deposit should be the StatusWaitPassthroughOrderComplete one
		require.Equal(t, di.DepositID, deposit.DepositID)
		require.Equal(t, StatusWaitSend, deposit.Status)
		require.Equal(t, c2cx.StatusCompleted.String(), deposit.Passthrough.Order.Status)
		require.Equal(t, fmt.Sprint(orderID), deposit.Passthrough.Order.OrderID)
		require.Equal(t, di.DepositID, deposit.Passthrough.Order.CustomerID)
		require.Equal(t, "0.00182", deposit.Passthrough.Order.Price)
		require.Equal(t, "1.23", deposit.Passthrough.Order.CompletedAmount)
		require.Equal(t, uint64(123e4), deposit.Passthrough.SkyBought)
		require.Equal(t, int64(223860), deposit.Passthrough.DepositValueSpent)
		require.True(t, deposit.Passthrough.Order.Final)
		require.NotEmpty(t, deposit.Passthrough.Order.Original)
		require.Empty(t, deposit.Error)

		// Check serialized original order

		deposit, err := p.store.(*Store).getDepositInfo(deposit.DepositID)
		require.NoError(t, err)

		var order c2cx.Order
		err = json.Unmarshal([]byte(deposit.Passthrough.Order.Original), &order)
		require.NoError(t, err)
		compareOrder(t, orderComplete, &order)
	}

	select {
	case <-p.Deposits():
		t.Fatal("Did not expect a deposit")
	default:
	}

	p.Shutdown()

	wg.Wait()
}

func TestPassthroughRetryRatelimited(t *testing.T) {
	// Tests that it retries if c2cx ratelimits the requests
	getOrderInfoErr := c2cx.NewAPIError("getorderinfo", http.StatusBadRequest, "Too Many Requests")
	testPassthroughRetryError(t, getOrderInfoErr)
}

func TestPassthroughRetryOtherError(t *testing.T) {
	// Tests that it retries if c2cx returns any other kind of error
	getOrderInfoErr := c2cx.NewOtherError(errors.New("unknown temporary failure"))
	testPassthroughRetryError(t, getOrderInfoErr)
}

func TestPassthroughRetryNetError(t *testing.T) {
	// Tests that it retries if c2cx returns net.Error
	getOrderInfoErr := &net.DNSError{}
	testPassthroughRetryError(t, getOrderInfoErr)
}

func TestPassthroughRetryInsufficientBalance(t *testing.T) {
	// Tests that it retries while balance is insufficient on the exchange
	p, shutdown, mockClient, _ := setupPassthrough(t)
	defer shutdown()

	di := createDepositStatusWaitPassthrough(t, p, testSkyAddr, 0)

	mockClient.On("GetOrderByStatus", c2cx.BtcSky, c2cx.StatusAll).Return(nil, nil)

	requestedAmount := calculateRequestedAmount(di.DepositValue)

	// First call will have insufficient balance
	mockClient.On("GetBalanceSummary").Return(&c2cx.BalanceSummary{
		Balance: c2cx.Balances{
			Btc: requestedAmount.Div(decimal.New(2, 0)),
		},
	}, nil).Once()

	// Second call will have sufficient balance
	mockClient.On("GetBalanceSummary").Return(&c2cx.BalanceSummary{
		Balance: c2cx.Balances{
			Btc: requestedAmount,
		},
	}, nil).Once()

	orderID := c2cx.OrderID(1234)

	mockClient.On("MarketBuy", c2cx.BtcSky, mock.MatchedBy(func(v decimal.Decimal) bool {
		return v.Equal(requestedAmount)
	}), &di.DepositID).Return(orderID, nil)

	orderComplete := &c2cx.Order{
		OrderID:         orderID,
		CustomerID:      &di.DepositID,
		Status:          c2cx.StatusCompleted,
		CompletedAmount: decimal.New(123, -2),
		AvgPrice:        decimal.New(182, -5),
	}

	// Return an order with a "complete" status
	mockClient.On("GetOrderInfo", c2cx.BtcSky, orderID).Return(orderComplete, nil).Once()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := p.Run()
		require.NoError(t, err)
	}()

	// Deposits should be found on .Deposits() in the correct order and with StatusWaitSend
	timeout := time.Second * 6
	select {
	case <-time.After(timeout):
		t.Fatal("Timed out waiting for the deposit to process")
	case deposit := <-p.Deposits():
		// The first deposit should be the StatusWaitPassthroughOrderComplete one
		require.Equal(t, di.DepositID, deposit.DepositID)
		require.Equal(t, StatusWaitSend, deposit.Status)
		require.Equal(t, c2cx.StatusCompleted.String(), deposit.Passthrough.Order.Status)
		require.Equal(t, fmt.Sprint(orderID), deposit.Passthrough.Order.OrderID)
		require.Equal(t, di.DepositID, deposit.Passthrough.Order.CustomerID)
		require.Equal(t, "0.00182", deposit.Passthrough.Order.Price)
		require.Equal(t, "1.23", deposit.Passthrough.Order.CompletedAmount)
		require.Equal(t, uint64(123e4), deposit.Passthrough.SkyBought)
		require.Equal(t, int64(223860), deposit.Passthrough.DepositValueSpent)
		require.True(t, deposit.Passthrough.Order.Final)
		require.NotEmpty(t, deposit.Passthrough.Order.Original)
		require.Empty(t, deposit.Error)

		// Check serialized original order

		deposit, err := p.store.(*Store).getDepositInfo(deposit.DepositID)
		require.NoError(t, err)

		var order c2cx.Order
		err = json.Unmarshal([]byte(deposit.Passthrough.Order.Original), &order)
		require.NoError(t, err)
		compareOrder(t, orderComplete, &order)
	}

	select {
	case <-p.Deposits():
		t.Fatal("Did not expect a deposit")
	default:
	}

	p.Shutdown()

	wg.Wait()
}

func TestPassthroughShutdownWhileCheckingOrderStatus(t *testing.T) {
	// Tests that passthrough shuts down safely while waiting to check order status
	// This will confirm quit error propagation handling
	p, shutdown, mockClient, _ := setupPassthrough(t)
	defer shutdown()

	p.cfg.C2CX.CheckOrderWait = time.Second * 60

	orderID := c2cx.OrderID(1234)
	di := createDepositStatusWaitPassthroughOrderComplete(t, p, testSkyAddr, 0, orderID)

	orderIncomplete := &c2cx.Order{
		OrderID:    orderID,
		CustomerID: &di.Passthrough.Order.CustomerID,
		Status:     c2cx.StatusPending,
	}

	mockClient.On("GetOrderByStatus", c2cx.BtcSky, c2cx.StatusAll).Return(nil, nil).Once()
	mockClient.On("GetOrderInfo", c2cx.BtcSky, orderID).Return(orderIncomplete, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := p.Run()
		require.NoError(t, err)
	}()

	// Wait enough time for the deposit to be in the step where it is waiting for the order status check
	time.Sleep(time.Second * 3)

	select {
	case <-p.Deposits():
		t.Fatal("Did not expect a deposit")
	default:
	}

	p.Shutdown()

	wg.Wait()

	// Check the deposit in the store
	deposit, err := p.store.(*Store).getDepositInfo(di.DepositID)
	require.NoError(t, err)

	require.Equal(t, di.DepositID, deposit.DepositID)
	require.Equal(t, StatusWaitPassthroughOrderComplete, deposit.Status)
	require.Empty(t, deposit.Passthrough.Order.Status)
	require.Equal(t, fmt.Sprint(orderID), deposit.Passthrough.Order.OrderID)
	require.Equal(t, di.DepositID, deposit.Passthrough.Order.CustomerID)
	require.Empty(t, deposit.Passthrough.Order.Price)
	require.Empty(t, deposit.Passthrough.Order.CompletedAmount)
	require.Equal(t, uint64(0), deposit.Passthrough.SkyBought)
	require.Equal(t, int64(0), deposit.Passthrough.DepositValueSpent)
	require.False(t, deposit.Passthrough.Order.Final)
	require.Empty(t, deposit.Passthrough.Order.Original)
	require.Empty(t, deposit.Error)
}

func TestPassthroughExchangeRunSend(t *testing.T) {
	// Tests an Exchanger configured with Passthrough end-to-end
	e, shutdown, _ := runExchange(t, config.BuyMethodPassthrough)
	defer shutdown()
	defer e.Shutdown()

	skyAddr := testSkyAddr
	btcAddr := "foo-btc-addr"
	_, err := e.store.BindAddress(skyAddr, btcAddr, config.CoinTypeBTC, config.BuyMethodPassthrough)
	require.NoError(t, err)

	dn := scanner.DepositNote{
		Deposit: scanner.Deposit{
			CoinType: config.CoinTypeBTC,
			Address:  btcAddr,
			Value:    134000,
			Height:   20,
			Tx:       "foo-tx",
			N:        2,
		},
		ErrC: make(chan error, 1),
	}

	orderID := c2cx.OrderID(1234)
	customerID := dn.Deposit.ID()
	orderComplete := &c2cx.Order{
		OrderID:         orderID,
		CustomerID:      &customerID,
		Status:          c2cx.StatusCompleted,
		CompletedAmount: decimal.New(123, -2),
		AvgPrice:        decimal.New(182, -5),
	}

	orderCompleteBytes, err := json.Marshal(orderComplete)
	require.NoError(t, err)

	requestedAmount := calculateRequestedAmount(dn.Deposit.Value)

	mockClient := e.Processor.(*Passthrough).exchangeClient.(*MockC2CXClient)
	mockClient.On("GetOrderByStatus", c2cx.BtcSky, c2cx.StatusAll).Return(nil, nil).Once()
	mockClient.On("GetBalanceSummary").Return(&c2cx.BalanceSummary{
		Balance: c2cx.Balances{
			Btc: decimal.New(5, 0),
		},
	}, nil).Once()
	mockClient.On("MarketBuy", c2cx.BtcSky, mock.MatchedBy(func(v decimal.Decimal) bool {
		return v.Equal(requestedAmount)
	}), &customerID).Return(orderID, nil)
	mockClient.On("GetOrderInfo", c2cx.BtcSky, orderID).Return(orderComplete, nil).Once()

	skySent, err := calculateSkyBought(orderComplete)
	require.NoError(t, err)
	txid := e.Sender.(*Send).sender.(*dummySender).predictTxid(t, skyAddr, skySent)

	mp := e.Receiver.(*Receive).multiplexer
	mp.GetScanner(config.CoinTypeBTC).(*dummyScanner).addDeposit(dn)

	// First loop calls saveIncomingDeposit
	// Then, the passthrough order is made and completes
	// Finally, the sender sends the coins then confirms them

	// nil is written to ErrC after saveIncomingDeposit finishes
	err = <-dn.ErrC
	require.NoError(t, err)

	// Periodically check the database until we observe the sent deposit
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range time.Tick(dbCheckWaitTime) {
			di, err := e.store.(*Store).getDepositInfo(dn.Deposit.ID())
			log.Printf("loop getDepositInfo %+v %v\n", di, err)
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
		CoinType:       config.CoinTypeBTC,
		UpdatedAt:      di.UpdatedAt,
		Status:         StatusWaitConfirm,
		SkyAddress:     skyAddr,
		DepositAddress: dn.Deposit.Address,
		DepositID:      dn.Deposit.ID(),
		Txid:           txid,
		SkySent:        skySent,
		BuyMethod:      config.BuyMethodPassthrough,
		ConversionRate: testSkyBtcRate,
		DepositValue:   dn.Deposit.Value,
		Deposit:        dn.Deposit,
		Passthrough: PassthroughData{
			ExchangeName:      PassthroughExchangeC2CX,
			RequestedAmount:   requestedAmount.String(),
			DepositValueSpent: calculateBtcSpent(orderComplete),
			SkyBought:         skySent,
			Order: PassthroughOrder{
				CustomerID:      customerID,
				OrderID:         fmt.Sprint(orderID),
				CompletedAmount: orderComplete.CompletedAmount.String(),
				Price:           orderComplete.AvgPrice.String(),
				Status:          orderComplete.Status.String(),
				Final:           true,
				Original:        string(orderCompleteBytes),
			},
		},
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
		CoinType:       config.CoinTypeBTC,
		UpdatedAt:      di.UpdatedAt,
		Status:         StatusDone,
		SkyAddress:     skyAddr,
		DepositAddress: dn.Deposit.Address,
		DepositID:      dn.Deposit.ID(),
		Txid:           txid,
		SkySent:        skySent,
		BuyMethod:      config.BuyMethodPassthrough,
		ConversionRate: testSkyBtcRate,
		DepositValue:   dn.Deposit.Value,
		Deposit:        dn.Deposit,
		Passthrough: PassthroughData{
			ExchangeName:      PassthroughExchangeC2CX,
			RequestedAmount:   requestedAmount.String(),
			DepositValueSpent: calculateBtcSpent(orderComplete),
			SkyBought:         skySent,
			Order: PassthroughOrder{
				CustomerID:      customerID,
				OrderID:         fmt.Sprint(orderID),
				CompletedAmount: orderComplete.CompletedAmount.String(),
				Price:           orderComplete.AvgPrice.String(),
				Status:          orderComplete.Status.String(),
				Final:           true,
				Original:        string(orderCompleteBytes),
			},
		},
	}

	require.Equal(t, expectedDeposit, di)
	closeMultiplexer(e)
}

func compareOrder(t *testing.T, a, b *c2cx.Order) {
	require.True(t, a.Amount.Equal(b.Amount))
	require.True(t, a.AvgPrice.Equal(b.AvgPrice))
	require.True(t, a.CompletedAmount.Equal(b.CompletedAmount))
	require.True(t, a.Fee.Equal(b.Fee))
	require.True(t, a.CreateDate.Equal(b.CreateDate))
	require.True(t, a.CompleteDate.Equal(b.CompleteDate))
	require.Equal(t, a.OrderID, b.OrderID)
	require.True(t, a.Price.Equal(b.Price))
	require.Equal(t, a.Status, b.Status)
	require.Equal(t, a.Type, b.Type)

	if a.Trigger == nil {
		require.Nil(t, b.Trigger)
	} else {
		require.True(t, a.Trigger.Equal(*b.Trigger))
	}

	if a.CustomerID == nil {
		require.Nil(t, b.CustomerID)
	} else {
		require.Equal(t, *a.CustomerID, *b.CustomerID)
	}

	require.Equal(t, a.Source, b.Source)
}

func TestCalculateRequestedAmount(t *testing.T) {
	cases := []struct {
		name string
		in   int64
		out  string
	}{
		{
			name: "zero",
			in:   0,
			out:  "0",
		},
		{
			name: "1 satoshis too small, truncated",
			in:   1,
			out:  "0",
		},
		{
			name: "10 satoshis too small, truncated",
			in:   10,
			out:  "0",
		},
		{
			name: "100 satoshis too small, truncated",
			in:   100,
			out:  "0",
		},
		{
			name: "1000 satoshis smallest value that won't truncate",
			in:   1000,
			out:  "0.00001",
		},
		{
			name: "10000 satoshis",
			in:   10000,
			out:  "0.0001",
		},
		{
			name: "100000 satoshis",
			in:   100000,
			out:  "0.001",
		},
		{
			name: "1000000 satoshis",
			in:   1000000,
			out:  "0.01",
		},
		{
			name: "10000000 satoshis (1 BTC)",
			in:   10000000,
			out:  "0.1",
		},
		{
			name: "100000000 satoshis (1 BTC)",
			in:   100000000,
			out:  "1",
		},
		{
			name: "mixed with truncation",
			in:   92045678111,
			out:  "920.45678",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			amt := calculateRequestedAmount(tc.in)
			require.Equal(t, tc.out, amt.String())
		})
	}
}

func TestCalculateSkyBought(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  uint64
		err  error
	}{
		{
			name: "negative amount",
			in:   "-1",
			err:  errCompletedAmountNegative,
		},
		{
			name: "one",
			in:   "1",
			out:  1e6,
		},
		{
			name: "fractional",
			in:   "1.23456",
			out:  1234560,
		},
		{
			name: "fractional exceeding 6 decimal places",
			in:   "1.23456789",
			out:  1234567,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			completedAmt, err := decimal.NewFromString(tc.in)
			require.NoError(t, err)

			amt, err := calculateSkyBought(&c2cx.Order{
				CompletedAmount: completedAmt,
			})

			if tc.err != nil {
				require.Equal(t, tc.err, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.out, amt)
		})
	}
}

func TestCalculateBtcSpent(t *testing.T) {
	cases := []struct {
		name   string
		bought string
		price  string
		out    int64
	}{
		{
			name:   "negative",
			bought: "-1",
			price:  "0.002",
			out:    -200000,
		},
		{
			name:   "zero bought",
			bought: "0",
			price:  "0.002",
			out:    0,
		},
		{
			name:   "zero price",
			bought: "1",
			price:  "0",
			out:    0,
		},
		{
			name:   "normal",
			bought: "32.43",
			price:  "0.00189",
			out:    6129270,
		},
		{
			name:   "normal",
			bought: "32.43",
			price:  "0.00189",
			out:    6129270,
		},
		{
			name:   "truncated",
			bought: "332.43",
			price:  "0.0018777",
			out:    62420381,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			completedAmt, err := decimal.NewFromString(tc.bought)
			require.NoError(t, err)

			avgPrice, err := decimal.NewFromString(tc.price)
			require.NoError(t, err)

			spent := calculateBtcSpent(&c2cx.Order{
				CompletedAmount: completedAmt,
				AvgPrice:        avgPrice,
			})
			require.Equal(t, tc.out, spent)
		})
	}
}
