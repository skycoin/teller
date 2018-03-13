package scanner

import (
	"fmt"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/skycoin/skycoin/src/visor"
	"github.com/skycoin/skycoin/src/api/webrpc"
	"github.com/skycoin/skycoin/src/api/cli"
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

// SkyRPCClient rpcclient interface
type SkyRPCClient interface {
	Send(recvAddr string, amount uint64) (string, error)
	GetTransaction(txid string) (*webrpc.TxnResult, error)
	GetBlocks(start, end uint64) (*visor.ReadableBlocks, error)
	GetBlocksBySeq(seq uint64) (*visor.ReadableBlock, error)
	GetLastBlocks() (*visor.ReadableBlock, error)
	Shutdown()
	SendBatch(saList []cli.SendAmount) (string, error)
}

// EthRPCClient rpcclient interface
type EthRPCClient interface {
	GetBlockVerboseTx(seq uint64) (*types.Block, error)
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
	CoinType  string // coin type
	Address   string // deposit address
	Value     int64  // deposit amount. For BTC, measured in satoshis.
	Height    int64  // the block height
	Tx        string // the transaction id
	N         uint32 // the index of vout in the tx [BTC]
	Processed bool   // whether this was received by the exchange and saved
}

// ID returns $tx:$n formatted ID string
func (d Deposit) ID() string {
	return fmt.Sprintf("%s:%d", d.Tx, d.N)
}

// GetCoinTypes returns supported coin types
func GetCoinTypes() []string {
	return []string{CoinTypeBTC, CoinTypeETH}
}
