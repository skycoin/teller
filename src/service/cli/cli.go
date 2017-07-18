package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"encoding/json"

	"github.com/skycoin/skycoin/src/cipher"
)

// Cli provides methods for sending coins
type Cli struct {
	wltFile string
	rpcAddr string
	envs    []string
}

// New creates Cli instance
func New(wltFile, rpcAddr string) *Cli {
	// // set ENV RPC_ADDR to localhost:7420, cause cli tool will use this env as rpc address
	// rpcAddrEnv := fmt.Sprintf("RPC_ADDR=%s", rpcAddr)

	if !strings.HasSuffix(wltFile, ".wlt") {
		panic("Invalid wallet file extension")
	}

	// test if the wallet exist
	if _, err := os.Stat(wltFile); os.IsNotExist(err) {
		panic(err)
	}

	return &Cli{
		wltFile: wltFile,
		rpcAddr: rpcAddr,
	}
}

// Send sends coins to recv address
func (c *Cli) Send(recvAddr string, amount int64) (string, error) {
	// validate the recvAddr
	if _, err := cipher.DecodeBase58Address(recvAddr); err != nil {
		return "", err
	}
	if amount < 1 {
		return "", fmt.Errorf("Can't send %d coins", amount)
	}

	amt := strconv.FormatInt(amount, 10)

	v, err := c.run(exec.Command("skycoin-cli", "send", "-j", "-f", c.wltFile, recvAddr, amt))
	if err != nil {
		return "", err
	}

	var rlt struct {
		Txid string `json:"txid"`
	}

	if err := json.NewDecoder(bytes.NewReader(v)).Decode(&rlt); err != nil {
		return "", fmt.Errorf("Cli send command failed, err: %s", string(v))
	}

	return rlt.Txid, nil
}

// TxJSON represents the json result of cli transaction
type TxJSON struct {
	Transaction struct {
		Status struct {
			Confirmed   bool `json:"confirmed"`
			Unconfirmed bool `json:"unconfirmed"`
			Height      int  `json:"height"`
			BlockSeq    int  `json:"block_seq"`
			Unknown     bool `json:"unknown"`
		} `json:"status"`
		Txn *json.RawMessage `json:"txn"`
	} `json:"transaction"`
}

// GetTransaction returns transaction by txid
func (c *Cli) GetTransaction(txid string) (*TxJSON, error) {
	v, err := c.run(exec.Command("skycoin-cli", "transaction", txid))
	if err != nil {
		return nil, err
	}

	var tx TxJSON
	if err := json.NewDecoder(bytes.NewReader(v)).Decode(&tx); err != nil {
		return nil, fmt.Errorf("Cli transaction command failed, err:%s", string(v))
	}

	return &tx, nil
}

func (c *Cli) run(cmd *exec.Cmd) ([]byte, error) {
	cmd.Env = append(os.Environ(), fmt.Sprintf("RPC_ADDR=%s", c.rpcAddr))
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return []byte{}, err
	}
	return out.Bytes(), nil
}
