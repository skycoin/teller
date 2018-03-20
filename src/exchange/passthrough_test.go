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
		di.Passthrough.RequestedAmount = calculateRequestedAmount(di).String()
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
