package rpc

import (
	"fmt"

	"github.com/skycoin/skycoin/src/api/cli"
	"github.com/skycoin/skycoin/src/api/webrpc"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/skycoin/src/wallet"
)

// RPC provides methods for sending coins
type RPC struct {
	walletFile string
	changeAddr string
	rpcClient  *webrpc.Client
}

// New creates RPC instance
func New(wltFile, rpcAddr string) *RPC {
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

	return &RPC{
		walletFile: wltFile,
		changeAddr: wlt.Entries[0].Address.String(),
		rpcClient:  rpcClient,
	}
}

// Send sends coins to recv address
func (c *RPC) Send(recvAddr string, amount uint64) (string, error) {
	// validate the recvAddr
	if _, err := cipher.DecodeBase58Address(recvAddr); err != nil {
		return "", err
	}

	if amount == 0 {
		return "", fmt.Errorf("Can't send 0 coins", amount)
	}

	sendAmount := cli.SendAmount{
		Addr:  recvAddr,
		Coins: amount,
	}

	return cli.SendFromWallet(c.rpcClient, c.walletFile, c.changeAddr, []cli.SendAmount{sendAmount})
}

// GetTransaction returns transaction by txid
func (c *RPC) GetTransaction(txid string) (*webrpc.TxnResult, error) {
	return c.rpcClient.GetTransactionByID(txid)
}
