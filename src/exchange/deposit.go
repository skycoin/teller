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

// Status deposit Status
type Status int8

const (
	// StatusWaitDeposit deposit has not occurred
	StatusWaitDeposit Status = iota
	// StatusWaitSend deposit is ready for send
	StatusWaitSend
	// StatusWaitConfirm coins sent, but not confirmed yet
	StatusWaitConfirm
	// StatusDone coins sent and confirmed
	StatusDone
	// StatusUnknown fallback value
	StatusUnknown
	// StatusWaitDecide initial deposit receive state
	StatusWaitDecide
	// StatusWaitPassthrough wait to buy from 3rd party exchange
	StatusWaitPassthrough

	// PassthroughExchangeC2CX for deposits using passthrough to c2cx.com
	PassthroughExchangeC2CX = "c2cx"
)

var statusString = []string{
	StatusWaitDeposit:     "waiting_deposit",
	StatusWaitSend:        "waiting_send",
	StatusWaitConfirm:     "waiting_confirm",
	StatusDone:            "done",
	StatusUnknown:         "unknown",
	StatusWaitDecide:      "waiting_decide",
	StatusWaitPassthrough: "waiting_passthrough",
}

func (s Status) String() string {
	return statusString[s]
}

// NewStatusFromStr create status from string
func NewStatusFromStr(st string) Status {
	switch st {
	case statusString[StatusWaitDeposit]:
		return StatusWaitDeposit
	case statusString[StatusWaitSend]:
		return StatusWaitSend
	case statusString[StatusWaitConfirm]:
		return StatusWaitConfirm
	case statusString[StatusDone]:
		return StatusDone
	case statusString[StatusWaitDecide]:
		return StatusWaitDecide
	case statusString[StatusWaitPassthrough]:
		return StatusWaitPassthrough
	default:
		return StatusUnknown
	}
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
	Seq            uint64
	UpdatedAt      int64
	Status         Status // TODO -- migrate to string statuses?
	CoinType       string
	SkyAddress     string
	BuyMethod      string
	DepositAddress string
	DepositID      string
	Txid           string
	ConversionRate string // SKY per other coin, as a decimal string (allows integers, floats, fractions)
	DepositValue   int64  // Deposit amount. Should be measured in the smallest unit possible (e.g. satoshis for BTC)
	SkySent        uint64 // SKY sent, measured in droplets
	Passthrough    PassthroughData
	Error          string // An error that occurred during processing
	// The original Deposit is saved for the records, in case there is a mistake.
	// Do not use this data directly.  All necessary data is copied to the top level
	// of DepositInfo (e.g. DepositID, DepositAddress, DepositValue, CoinType).
	Deposit scanner.Deposit
}

// PassthroughData encapsulates data used for OTC passthrough
type PassthroughData struct {
	ExchangeName      string
	SkyBought         uint64
	DepositValueSpent int64
	Orders            []PassthroughOrder
}

// PassthroughOrder contains information about an order placed on an exchange for passthrough
type PassthroughOrder struct {
	ID        string
	Amount    string
	Price     string
	Timestamp string
	CoinType  string
}

// DepositStats records overall statistics about deposits
type DepositStats struct {
	TotalBTCReceived int64 `json:"total_btc_received"`
	TotalSKYSent     int64 `json:"total_sky_sent"`
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
		if di.CoinType == scanner.CoinTypeBTC && !isValidBtcTx(di.DepositID) {
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

	case StatusWaitDeposit, StatusUnknown:
		fallthrough
	default:
		return fmt.Errorf("DepositInfo should not have status %s[%d]", di.Status.String(), di.Status)
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
