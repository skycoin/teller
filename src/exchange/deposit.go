package exchange

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/skycoin/teller/src/config"
	"github.com/skycoin/teller/src/scanner"
	"github.com/skycoin/teller/src/util/mathutil"
)

const (
	// StatusWaitDeposit deposit has not occurred
	StatusWaitDeposit = "waiting_deposit"
	// StatusWaitDecide initial deposit receive state
	StatusWaitDecide = "waiting_decide"
	// StatusWaitSend deposit is ready for send
	StatusWaitSend = "waiting_send"
	// StatusWaitConfirm coins sent, but not confirmed yet
	StatusWaitConfirm = "waiting_confirm"
	// StatusWaitPassthrough wait to buy from 3rd party exchange
	StatusWaitPassthrough = "waiting_passthrough"
	// StatusWaitPassthroughOrderComplete wait for order placed on 3rd party exchange to complete
	StatusWaitPassthroughOrderComplete = "waiting_passthrough_order_complete"
	// StatusDone coins sent and confirmed
	StatusDone = "done"
	// StatusUnknown fallback value
	StatusUnknown = "unknown"

	// PassthroughExchangeC2CX for deposits using passthrough to c2cx.com
	PassthroughExchangeC2CX = "c2cx"
)

var (
	// ErrInvalidStatus is returned for invalid statuses
	ErrInvalidStatus = errors.New("Invalid status")

	// Statuses is all valid statuses
	Statuses = []string{
		StatusWaitDeposit,
		StatusWaitSend,
		StatusWaitConfirm,
		StatusDone,
		StatusWaitDecide,
		StatusWaitPassthrough,
		StatusWaitPassthroughOrderComplete,
	}
)

// ValidateStatus returns an error if a status is invalid
func ValidateStatus(s string) error {
	for _, k := range Statuses {
		if k == s {
			return nil
		}
	}

	return ErrInvalidStatus
}

// BoundAddress records information about an address binding
type BoundAddress struct {
	SkyAddress string
	Address    string
	CoinType   string
	BuyMethod  string
}

// DepositInfo records the deposit info
type DepositInfo struct {
	Seq            uint64          `json:"seq"`
	UpdatedAt      int64           `json:"updated_at"`
	Status         string          `json:"status"`
	CoinType       string          `json:"coin_type"`
	SkyAddress     string          `json:"sky_address"`
	BuyMethod      string          `json:"buy_method"`
	DepositAddress string          `json:"deposit_address"`
	DepositID      string          `json:"deposit_id"`
	Txid           string          `json:"txid"`
	ConversionRate string          `json:"conversion_rate"` // SKY per other coin, as a decimal string (allows integers, floats, fractions)
	DepositValue   int64           `json:"deposit_value"`   // Deposit amount. Should be measured in the smallest unit possible (e.g. satoshis for BTC)
	SkySent        uint64          `json:"sky_sent"`        // SKY sent, measured in droplets
	Passthrough    PassthroughData `json:"passthrough"`
	Error          string          `json:"error"` // An error that occurred during processing
	// The original Deposit is saved for the records, in case there is a mistake.
	// Do not use this data directly.  All necessary data is copied to the top level
	// of DepositInfo (e.g. DepositID, DepositAddress, DepositValue, CoinType).
	Deposit scanner.Deposit `json:"deposit"`
}

// PassthroughData encapsulates data used for OTC passthrough
type PassthroughData struct {
	ExchangeName      string           `json:"exchange_name"`
	SkyBought         uint64           `json:"sky_bought"`
	DepositValueSpent int64            `json:"deposit_value_spent"`
	RequestedAmount   string           `json:"requested_amount"`
	Order             PassthroughOrder `json:"order"`
}

// PassthroughOrder encapsulates 3rd party exchange order data
type PassthroughOrder struct {
	CustomerID      string `json:"customer_id"`
	OrderID         string `json:"order_id"`
	CompletedAmount string `json:"completed_amount"`
	Price           string `json:"price"`
	Status          string `json:"status"`
	Final           bool   `json:"final"`
	Original        string `json:"original"`
}

// DepositStats records overall statistics about deposits
type DepositStats struct {
	Received map[string]int64 `json:"received"`
	Sent     int64            `json:"sent"`
}

// ValidateForStatus does a consistency check of the data based upon the Status value
func (di DepositInfo) ValidateForStatus() error {

	checkWaitSend := func() error {
		if di.Seq == 0 {
			return errors.New("Seq missing")
		}
		if di.SkyAddress == "" {
			return errors.New("SkyAddress missing")
		}
		if di.DepositAddress == "" {
			return errors.New("DepositAddress missing")
		}
		if di.DepositID == "" {
			return errors.New("DepositID missing")
		}
		if di.CoinType == config.CoinTypeBTC && !isValidBtcTx(di.DepositID) {
			return fmt.Errorf("Invalid DepositID value \"%s\"", di.DepositID)
		}
		if di.DepositValue == 0 {
			return errors.New("DepositValue is zero")
		}
		if _, err := mathutil.ParseRate(di.ConversionRate); err != nil {
			return err
		}
		switch di.BuyMethod {
		case config.BuyMethodDirect, config.BuyMethodPassthrough:
		case "":
			return errors.New("BuyMethod missing")
		default:
			return errors.New("BuyMethod invalid")
		}

		return nil
	}

	switch di.Status {
	case StatusDone:
		if di.Error != ErrEmptySendAmount.Error() && di.Txid == "" {
			return errors.New("Txid missing")
		}
		// Don't check SkySent == 0, it is possible to have StatusDone with
		// no sky sent, this happens if we received a very tiny deposit.
		// In that case, the DepositInfo status is switched to StatusDone
		// without doing StatusWaitConfirm
		return checkWaitSend()

	case StatusWaitConfirm:
		if di.Txid == "" {
			return errors.New("Txid missing")
		}
		if di.SkySent == 0 {
			return errors.New("SkySent is zero")
		}
		return checkWaitSend()

	case StatusWaitSend:
		return checkWaitSend()

	case StatusWaitDecide:
		return checkWaitSend()

	case StatusWaitPassthroughOrderComplete:
		if di.Passthrough.Order.OrderID == "" {
			return errors.New("Passthrough.Order.OrderID missing")
		}
		fallthrough

	case StatusWaitPassthrough:
		if di.Passthrough.ExchangeName == "" {
			return errors.New("Passthrough.ExchangeName missing")
		}
		if di.Passthrough.RequestedAmount == "" {
			return errors.New("Passthrough.RequestedAmount missing")
		}
		if di.Passthrough.Order.CustomerID == "" {
			return errors.New("Passthrough.Order.CustomerID missing")
		}

		return checkWaitSend()

	case "":
		return errors.New("DepositInfo is missing status")

	case StatusWaitDeposit, StatusUnknown:
		fallthrough
	default:
		return fmt.Errorf("DepositInfo should not have status %s", di.Status)
	}
}

func isValidBtcTx(btcTx string) bool {
	if btcTx == "" {
		return false
	}

	pts := strings.Split(btcTx, ":")
	if len(pts) != 2 {
		return false
	}

	if pts[0] == "" || pts[1] == "" {
		return false
	}

	_, err := strconv.ParseInt(pts[1], 10, 64)
	return err == nil
}
