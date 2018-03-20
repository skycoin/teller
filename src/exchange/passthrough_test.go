package exchange

import (
	"errors"
	"fmt"
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

func createDepositStatusWaitDecide(t *testing.T, p *Passthrough) DepositInfo {
	_, err := p.store.BindAddress(testSkyAddr, "btc-address", scanner.CoinTypeBTC, config.BuyMethodPassthrough)
	require.NoError(t, err)

	depositInfo, err := p.store.GetOrCreateDepositInfo(scanner.Deposit{
		CoinType:  scanner.CoinTypeBTC,
		Address:   "btc-address",
		Value:     1000000,
		Height:    400000,
		Tx:        "btc-tx-id",
		N:         1,
		Processed: true,
	}, defaultPassthroughCfg.SkyBtcExchangeRate)
	require.NoError(t, err)

	return depositInfo
}

func createDepositStatusWaitPassthrough(t *testing.T, p *Passthrough) DepositInfo {
	depositInfo := createDepositStatusWaitDecide(t, p)

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

func createDepositStatusWaitPassthroughOrderComplete(t *testing.T, p *Passthrough, orderID c2cx.OrderID) DepositInfo {
	depositInfo := createDepositStatusWaitPassthrough(t, p)

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

	createDepositStatusWaitPassthrough(t, p)

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

	di := createDepositStatusWaitPassthrough(t, p)
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
