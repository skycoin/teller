package exchange

import "github.com/skycoin/teller/src/scanner"

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
	Seq        uint64
	UpdatedAt  int64
	Status     Status
	SkyAddress string
	BtcAddress string
	BtcTx      string
	Txid       string
	SkySent    uint64 // SKY sent, measured in droplets
	SkyBtcRate int64  // SKY per BTC rate
	Deposit    scanner.Deposit
}
