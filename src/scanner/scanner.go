package scanner

import (
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// Scanner provids apis for interacting with a scan service
type Scanner interface {
	AddScanAddress(string) error
	GetScanAddresses() ([]string, error)
	GetDepositValue() <-chan DepositNote
	Run() error
	Shutdown()
}

// BtcRPCClient rpcclient interface
type BtcRPCClient interface {
	GetBestBlock() (*chainhash.Hash, int32, error)
	GetBlockVerboseTx(blockHash *chainhash.Hash) (*btcjson.GetBlockVerboseResult, error)
	Shutdown()
}

// DepositNote wraps a DepositValue with an ack channel
type DepositNote struct {
	DepositValue
	AckC chan struct{}
}

func makeDepositNote(dv DepositValue) DepositNote {
	return DepositNote{
		DepositValue: dv,
		AckC:         make(chan struct{}, 1),
	}
}

// Config scanner config info
type Config struct {
	ScanPeriod time.Duration // scan period in seconds
}

// DepositValue struct
type DepositValue struct {
	Address string // deposit address
	Value   int64  // deposit amount. For BTC, measured in satoshis.
	Height  int64  // the block height
	Tx      string // the transaction id
	N       uint32 // the index of vout in the tx
	IsUsed  bool   // whether this dv is used
}

// TxN returns $tx:$n formatted ID string
func (d DepositValue) TxN() string {
	return fmt.Sprintf("%s:%d", d.Tx, d.N)
}
