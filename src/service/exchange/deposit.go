package exchange

// status deposit status
type status int8

const (
	statusWaitDeposit status = iota
	statusWaitSend
	statusWaitConfirm
	statusDone
)

var statusString = []string{
	statusWaitDeposit: "waiting_deposit",
	statusWaitSend:    "waiting_send",
	statusWaitConfirm: "waiting_confirm",
	statusDone:        "done",
}

func (s status) String() string {
	return statusString[s]
}

// depositInfo records the deposit info
type depositInfo struct {
	Seq        uint64
	UpdatedAt  int64
	Status     status
	SkyAddress string
	BtcAddress string
	Txid       string
}

// only part of the variable are mofiy allowed
func (dpi *depositInfo) updateMutableVar(newDpi depositInfo) {
	dpi.Status = newDpi.Status
	dpi.Txid = newDpi.Txid
	dpi.UpdatedAt = newDpi.UpdatedAt
}
