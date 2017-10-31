package sender

import (
	"errors"

	"github.com/skycoin/skycoin/src/api/cli"
	"github.com/skycoin/skycoin/src/api/webrpc"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/skycoin/src/coin"
	"github.com/skycoin/skycoin/src/wallet"
)

// RPC provides methods for sending coins
type RPC struct {
	walletFile string
	changeAddr string
	rpcClient  *webrpc.Client
}

// NewRPC creates RPC instance
func NewRPC(wltFile, rpcAddr string) (*RPC, error) {
	wlt, err := wallet.Load(wltFile)
	if err != nil {
		return nil, err
	}

	if len(wlt.Entries) == 0 {
		return nil, errors.New("Wallet is empty")
	}

	rpcClient := &webrpc.Client{
		Addr: rpcAddr,
	}

	return &RPC{
		walletFile: wltFile,
		changeAddr: wlt.Entries[0].Address.String(),
		rpcClient:  rpcClient,
	}, nil
}

// CreateTransaction creates a raw Skycoin transaction offline, that can be broadcast later
func (c *RPC) CreateTransaction(recvAddr string, amount uint64) (*coin.Transaction, error) {
	// TODO -- this can support sending to multiple receivers at once,
	// which would be necessary if the exchange was busy
	sendAmount := cli.SendAmount{
		Addr:  recvAddr,
		Coins: amount,
	}

	if err := validateSendAmount(sendAmount); err != nil {
		return nil, err
	}

	return cli.CreateRawTxFromWallet(c.rpcClient, c.walletFile, c.changeAddr, []cli.SendAmount{sendAmount})
}

// BroadcastTransaction broadcasts a transaction and returns its txid
func (c *RPC) BroadcastTransaction(tx *coin.Transaction) (string, error) {
	return c.rpcClient.InjectTransaction(tx)
}

// GetTransaction returns transaction by txid
func (c *RPC) GetTransaction(txid string) (*webrpc.TxnResult, error) {
	return c.rpcClient.GetTransactionByID(txid)
}

func validateSendAmount(amt cli.SendAmount) error {
	// validate the recvAddr
	if _, err := cipher.DecodeBase58Address(amt.Addr); err != nil {
		return err
	}

	if amt.Coins == 0 {
		return errors.New("Can't send 0 coins")
	}

	return nil
}
