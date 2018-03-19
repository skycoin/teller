package webrpc

import (
	"github.com/MDLlife/MDL/src/cipher"
	"github.com/MDLlife/MDL/src/coin"
	"github.com/MDLlife/MDL/src/daemon"
	"github.com/MDLlife/MDL/src/visor"
	"github.com/MDLlife/MDL/src/visor/historydb"
)

//go:generate goautomock -template=testify Gatewayer

// Gatewayer provides interfaces for getting mdl related info.
type Gatewayer interface {
	GetLastBlocks(num uint64) (*visor.ReadableBlocks, error)
	GetBlocks(start, end uint64) (*visor.ReadableBlocks, error)
	GetBlocksInDepth(vs []uint64) (*visor.ReadableBlocks, error)
	GetUnspentOutputs(filters ...daemon.OutputsFilter) (*visor.ReadableOutputSet, error)
	GetTransaction(txid cipher.SHA256) (*visor.Transaction, error)
	InjectBroadcastTransaction(tx coin.Transaction) error
	GetAddrUxOuts(addr cipher.Address) ([]*historydb.UxOutJSON, error)
	GetTimeNow() uint64
}
