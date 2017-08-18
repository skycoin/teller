package rpc

import (
	"fmt"

	"github.com/skycoin/skycoin/src/api/cli"
	"github.com/skycoin/skycoin/src/api/webrpc"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/skycoin/src/wallet"
)

// Rpc provides methods for sending coins
type Rpc struct {
	walletFile string
	changeAddr string
	rpcClient  *webrpc.Client
}

// New creates Rpc instance
func New(wltFile, rpcAddr string) *Rpc {
	wlt, err := wallet.Load(wltFile)
	if err != nil {
		panic(err)
	}

	if len(wlt.Entries) == 0 {
		panic("Wallet is empty")
	}

	rpcClient := &webrpc.Client{
		Addr: rpcAddr,
	}

	return &Rpc{
		walletFile: wltFile,
		changeAddr: wlt.Entries[0].Address.String(),
		rpcClient:  rpcClient,
	}
}

// Send sends coins to recv address
func (c *Rpc) Send(recvAddr string, amount int64) (string, error) {
	// validate the recvAddr
	if _, err := cipher.DecodeBase58Address(recvAddr); err != nil {
		return "", err
	}
	if amount < 1 {
		return "", fmt.Errorf("Can't send %d coins", amount)
	}

	sendAmount := cli.SendAmount{
		Addr:  recvAddr,
		Coins: uint64(amount),
	}

	return cli.SendFromWallet(c.rpcClient, c.walletFile, c.changeAddr, []cli.SendAmount{sendAmount})
}

// GetTransaction returns transaction by txid
func (c *Rpc) GetTransaction(txid string) (*webrpc.TxnResult, error) {
	return c.rpcClient.GetTransactionByID(txid)
}
