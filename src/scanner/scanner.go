package scanner

import (
	"fmt"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/skycoin/skycoin/src/visor"
)

// Scanner provids apis for interacting with a scan service
type Scanner interface {
	AddScanAddress(string, string) error
	GetDeposit() <-chan DepositNote
}

// BtcRPCClient rpcclient interface
type BtcRPCClient interface {
	GetBlockVerboseTx(*chainhash.Hash) (*btcjson.GetBlockVerboseResult, error)
	GetBlockHash(int64) (*chainhash.Hash, error)
	GetBlockCount() (int64, error)
	Shutdown()
}

// EthRPCClient rpcclient interface
type EthRPCClient interface {
	GetBlockVerboseTx(seq uint64) (*types.Block, error)
	GetBlockCount() (int64, error)
	Shutdown()
}

// SkyRPCClient rpcclient interface
// required so that we can mock it for testing
type SkyRPCClient interface {
	GetBlockVerboseTx(seq uint64) (*visor.ReadableBlock, error)
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
	CoinType  string `json:"coin_type"` // coin type
	Address   string `json:"address"`   // deposit address
	Value     int64  `json:"value"`     // deposit amount. For BTC, measured in satoshis.
	Height    int64  `json:"height"`    // the block height
	Tx        string `json:"tx"`        // the transaction id
	N         uint32 `json:"n"`         // the index of vout in the tx [BTC]
	Processed bool   `json:"processed"` // whether this was received by the exchange and saved
}

// ID returns $tx:$n formatted ID string
func (d Deposit) ID() string {
	return fmt.Sprintf("%s:%d", d.Tx, d.N)
}
