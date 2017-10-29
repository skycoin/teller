package scanner

import (
	"fmt"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// Scanner provids apis for interacting with a scan service
type Scanner interface {
	AddScanAddress(string) error
	GetScanAddresses() ([]string, error)
	GetDeposit() <-chan DepositNote
}

// BtcRPCClient rpcclient interface
type BtcRPCClient interface {
	GetBlockVerboseTx(*chainhash.Hash) (*btcjson.GetBlockVerboseResult, error)
	GetBlockHash(int64) (*chainhash.Hash, error)
	GetBlockCount() (int64, error)
	Shutdown()
}

// DepositNote wraps a Deposit with an ack channel
type DepositNote struct {
	Deposit
	ErrC chan error
}

// NewDepositNote returns a DepositNote
func NewDepositNote(dv Deposit) DepositNote {
	return DepositNote{
		Deposit: dv,
		ErrC:    make(chan error, 1),
	}
}

// Deposit struct
type Deposit struct {
	Address   string // deposit address
	Value     int64  // deposit amount. For BTC, measured in satoshis.
	Height    int64  // the block height
	Tx        string // the transaction id
	N         uint32 // the index of vout in the tx
	Processed bool   // whether this was received by the exchange and saved
}

// TxN returns $tx:$n formatted ID string
func (d Deposit) TxN() string {
	return fmt.Sprintf("%s:%d", d.Tx, d.N)
}
