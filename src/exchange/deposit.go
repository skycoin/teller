package exchange

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/skycoin/teller/src/scanner"
)

// Status deposit Status
type Status int8

const (
	// StatusWaitDeposit deposit has not occured
	StatusWaitDeposit Status = iota
	// StatusWaitSend deposit received, but send has not occured
	StatusWaitSend
	// StatusWaitConfirm coins sent, but not confirmed yet
	StatusWaitConfirm
	// StatusDone coins sent and confirmed
	StatusDone
	// StatusUnknown fallback value
	StatusUnknown
)

var statusString = []string{
	StatusWaitDeposit: "waiting_deposit",
	StatusWaitSend:    "waiting_send",
	StatusWaitConfirm: "waiting_confirm",
	StatusDone:        "done",
	StatusUnknown:     "unknown",
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
	default:
		return StatusUnknown
	}
}

// DepositInfo records the deposit info
type DepositInfo struct {
	Seq            uint64
	UpdatedAt      int64
	Status         Status
	CoinType       string
	SkyAddress     string
	DepositAddress string
	DepositID      string
	Txid           string
	ConversionRate int64  // Number of whole SKY per other whole coin
	DepositValue   int64  // Deposit amount. Should be measured in the smallest unit possible (e.g. satoshis for BTC)
	SkySent        uint64 // SKY sent, measured in droplets
	// The original Deposit is saved for the records, in case there is a mistake.
	// Do not use this data directly.  All necessary data is copied to the top level
	// of DepositInfo (e.g. DepositID, DepositAddress, DepositValue, CoinType).
	Deposit scanner.Deposit
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
		if di.ConversionRate == 0 {
			return errors.New("ConversionRate is zero")
		}
		return nil
	}

	switch di.Status {
	case StatusDone:
		if di.Txid == "" {
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
	if err != nil {
		return false
	}

	return true
}
