package exchange

// Status deposit Status
type Status int8

const (
	StatusWaitDeposit Status = iota
	StatusWaitSend
	StatusWaitConfirm
	StatusDone
	StatusUnknown
)

var statusString = []string{
	StatusWaitDeposit: "waiting_deposit",
	StatusWaitSend:    "waiting_send",
	StatusWaitConfirm: "waiting_confirm",
	StatusDone:        "done",
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
}
