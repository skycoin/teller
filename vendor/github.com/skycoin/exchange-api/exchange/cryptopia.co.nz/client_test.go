// +build cryptopia_integration_test

package cryptopia

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/skycoin/exchange-api/db"
	"github.com/skycoin/exchange-api/exchange"
)

var c = Client{
	Key:                      key,
	Secret:                   secret,
	Orders:                   exchange.NewTracker(),
	OrdersRefreshInterval:    time.Millisecond * 500,
	OrderbookRefreshInterval: time.Second * 5,
	Stop: make(chan struct{}),
}

func TestClientGetBalance(t *testing.T) {

	str, err := c.GetBalance("BTC")
	if err != nil {
		t.Fatal(err)
	}
	if str != "Total: 0.00000000 Available: 0.00000000 Unconfirmed: 0.00000000 Held: 0.00000000 Pending: 0.00000000" {
		t.Log("You has money :)")
	}
}

func TestClientCancel(t *testing.T) {
	_, err := c.Cancel(1)
	if err == nil {
		t.Log("whoops")
	}
}
func TestClientCancelMarket(t *testing.T) {
	orders, err := c.CancelMarket("LTC/BTC")
	if len(orders) > 0 {
		t.Fatalf("Unexpected ordres %v", orders)
	}
	if err == nil {
		t.Log("whoops")
	}
}

func TestClientCancelAll(t *testing.T) {
	orders, err := c.CancelAll()
	if len(orders) > 0 {
		t.Fatalf("Unexpected orders %v", orders)
	}
	if err == nil {
		t.Log("whoops")
	}
}
func TestClientBuy(t *testing.T) {
	one := decimal.New(1, 0)
	order, err := c.Buy("LTC/BTC", one, one)
	if err == nil {
		t.Log("order successfully created")
		if order == 0 {
			t.Log("order executed instantly")
		}
	}
}
func TestClientSell(t *testing.T) {
	one := decimal.New(1, 0)
	order, err := c.Sell("LTC/BTC", one, one)
	if err == nil {
		t.Log("order successfully created")
		if order == 0 {
			t.Log("order executed instantly")
		}
	}
}

/*
func TestClientExecuted(t *testing.T) {
	c.Tracker.NewOrder("LTC/BTC", exchange.ActionBuy, exchange.StatusOpened, 1, 10, 0.1)
	if len(c.Executed()) != 1 {
		t.Fatal("order does not added")
	}
}

func TestClientOrderStatus(t *testing.T) {
	accepted := time.Now()
	c.Tracker.UpdateOrderDetails(1, exchange.StatusPartial, &accepted)
	status, err := c.OrderStatus(1)
	if status != exchange.StatusPartial || err != nil {
		t.Fatal(status, err)
	}
}

func TestClientCompleted(t *testing.T) {
	c.Tracker.Complete(1, time.Now())
	if len(c.Completed()) != 1 {
		t.Fatal("order incompleted")
	}
}

func TestClientOrderDetails(t *testing.T) {
	info, err := c.OrderDetails(1)
	if err != nil {
		t.Fatal(info, err)
	}
}
*/

func TestClientUpdateOrderbook(t *testing.T) {
	orderBook, err := db.NewOrderbookTracker()

	if err != nil {
		t.Fatal(err)
	}

	var c = Client{
		Key: "", Secret: "",
		Orderbooks:               orderBook,
		TrackedBooks:             []string{"LTC/BTC"},
		OrderbookRefreshInterval: time.Second * 5,
	}

	c.updateOrderbook()
	if _, err = c.Orderbook().Get("LTC_BTC"); err != nil {
		t.Fatal(err)
	}
}
