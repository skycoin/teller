package exchange

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
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
		MaxDecimals:             3,
		TxConfirmationCheckWait: time.Second,
		Wallet:                  testWalletFile,
		SendEnabled:             false,
		BuyMethod:               config.BuyMethodPassthrough,
		C2CX: config.C2CX{
			Key:                "c2cx-key",
			Secret:             "c2cx-secret",
			RequestFailureWait: time.Millisecond * 30,
			RatelimitWait:      time.Millisecond * 60,
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
	_, err := p.store.BindAddress(skyAddr, btcAddr, scanner.CoinTypeBTC, config.BuyMethodPassthrough)
	require.NoError(t, err)

	depositInfo, err := p.store.GetOrCreateDepositInfo(scanner.Deposit{
		CoinType:  scanner.CoinTypeBTC,
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

func TestPassthroughStartupFailure(t *testing.T) {
	// Tests what happens when Run() initialization fails
	// If there are any StatusWaitPassthrough deposits, fixUnrecordedOrders()
	// will try to match them by CustomerID to orders in our orderbook on the exchange.
	// When it calls GetOrderByStatus, fail, and check that Run() exits with this error.

	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)
	store, err := NewStore(log, db)
	require.NoError(t, err)

	receiver := newMockReceiver()

	p, err := NewPassthrough(log, defaultPassthroughCfg, store, receiver)
	require.NoError(t, err)
	require.False(t, p.exchangeClient.(*c2cx.Client).Debug, "c2cx client debug should be off")

	mockClient := &MockC2CXClient{}
	p.exchangeClient = mockClient

	getOrderByStatusErr := errors.New("GetOrderByStatus failure")
	mockClient.On("GetOrderByStatus", c2cx.BtcSky, c2cx.StatusAll).Return(nil, getOrderByStatusErr)

	createDepositStatusWaitPassthrough(t, p, testSkyAddr, 0)

	err = p.Run()
	require.Error(t, err)
	require.Equal(t, getOrderByStatusErr, err)
}

func TestPassthroughFixUnrecordedOrders(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)
	store, err := NewStore(log, db)
	require.NoError(t, err)

	receiver := newMockReceiver()

	p, err := NewPassthrough(log, defaultPassthroughCfg, store, receiver)
	require.NoError(t, err)

	mockClient := &MockC2CXClient{}
	p.exchangeClient = mockClient

	di := createDepositStatusWaitPassthrough(t, p, testSkyAddr, 0)
	require.NotEmpty(t, di.Passthrough.Order.CustomerID)

	orderID := 1234
	otherCID := "abcd"

	orders := []c2cx.Order{
		{
			OrderID: c2cx.OrderID(orderID),
			CID:     &otherCID,
		},
		{
			OrderID: c2cx.OrderID(orderID + 1),
			CID:     nil,
		},
		{
			OrderID: c2cx.OrderID(orderID + 2),
			CID:     &di.Passthrough.Order.CustomerID,
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

func TestLoadExistingDeposits(t *testing.T) {
	// Tests that existing StatusWaitPassthrough and StatusWaitPassthroughOrderComplete
	// deposits are loaded and processed.
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)
	store, err := NewStore(log, db)
	require.NoError(t, err)

	receiver := newMockReceiver()

	p, err := NewPassthrough(log, defaultPassthroughCfg, store, receiver)
	require.NoError(t, err)

	mockClient := &MockC2CXClient{}
	p.exchangeClient = mockClient

	// Create a StatusWaitPassthrough and StatusWaitPassthroughOrderComplete
	orderIDWaitPassthrough := c2cx.OrderID(1234)
	orderIDWaitPassthroughOrderComplete := c2cx.OrderID(1235)

	diWaitPassthrough := createDepositStatusWaitPassthrough(t, p, testSkyAddr, 0)
	diWaitPassthrough, err = p.store.UpdateDepositInfo(diWaitPassthrough.DepositID, func(di DepositInfo) DepositInfo {
		di.DepositValue = 7e5
		di.Passthrough.RequestedAmount = "0.007"
		return di
	})
	require.NoError(t, err)

	diWaitPassthroughOrderComplete := createDepositStatusWaitPassthroughOrderComplete(t, p, testSkyAddr2, 1, orderIDWaitPassthroughOrderComplete)

	orderWaitPassthrough := &c2cx.Order{
		OrderID:         orderIDWaitPassthrough,
		CID:             &diWaitPassthrough.Passthrough.Order.CustomerID,
		Status:          c2cx.StatusCompleted,
		CompletedAmount: decimal.New(123, -2),
		AvgPrice:        decimal.New(182, -5),
	}

	orderWaitPassthroughOrderComplete := &c2cx.Order{
		OrderID:         orderIDWaitPassthroughOrderComplete,
		CID:             &diWaitPassthroughOrderComplete.Passthrough.Order.CustomerID,
		Status:          c2cx.StatusCompleted,
		CompletedAmount: decimal.New(345, -2),
		AvgPrice:        decimal.New(201, -5),
	}

	// GetOrderByStatus should return no matching orders
	mockClient.On("GetOrderByStatus", c2cx.BtcSky, c2cx.StatusAll).Return(nil, nil)

	mockClient.On("MarketBuy", c2cx.BtcSky, decimal.New(7, -3), &diWaitPassthrough.Passthrough.Order.CustomerID).Return(orderIDWaitPassthrough, nil)

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
		require.NotNil(t, deposit.Passthrough.Order.Original)
		require.IsType(t, c2cx.Order{}, deposit.Passthrough.Order.Original)
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
		require.NotNil(t, deposit.Passthrough.Order.Original)
		require.IsType(t, c2cx.Order{}, deposit.Passthrough.Order.Original)
	}

	select {
	case <-p.Deposits():
		t.Fatal("Did not expect a deposit")
	default:
	}

	p.Shutdown()

	wg.Wait()
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
			out:  "0.1",
		},
		{
			name: "10000000 satoshis (1 BTC)",
			in:   10000000,
			out:  "1",
		},
		{
			name: "mixed with truncation",
			in:   92045678111,
			out:  "92.04567",
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
			out:    200000,
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
