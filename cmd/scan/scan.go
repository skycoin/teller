package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	_"log"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcutil"
	"strconv"
)

type Address struct {
	Addr         string `json:"address"`
	MinScanBlock int64  `json:"min_scan_block"`
	MidScanBlock int64  `json:"mid_scan_block"`
	MaxScanBlock int64  `json:"max_scan_block"`
	Txs          []Tx   `json:"txs"`
}

type Tx struct {
	TxHash      string `json:"tx_hash"`
	BlockHash   string `json:"block_hash"`
	ParentHash  string `json:"parent_hash"`
	BlockHeight int64  `json:"block_height"`
}

type Deposit struct {
	Addr string
	Tx   Tx
}

func ScanBlock(client *rpcclient.Client, blockID int64) ([]Deposit, error) {
	blockHash, err := client.GetBlockHash(blockID)
	if err != nil {
		return nil, err
	}
	block, err := client.GetBlockVerboseTx(blockHash)
	if err != nil {
		return nil, err
	}
	parentHash := block.PreviousHash
	var deposits []Deposit
	var depTx Tx
	depTx.BlockHeight = blockID
	depTx.BlockHash = blockHash.String()
	depTx.ParentHash = parentHash
	for _, tx := range block.RawTx {
		for i, addr := range tx.Vout {
			depTx.TxHash = tx.Txid
			depTx.TxHash = depTx.TxHash + ":" + strconv.Itoa(i)
			if len(addr.ScriptPubKey.Addresses) > 0 {
				deposits = append(deposits, Deposit{Addr: addr.ScriptPubKey.Addresses[0], Tx: depTx})
			}
		}

	}
	return deposits, nil
}

func CompareAddress(addr Address, deps []Deposit) Address {
	for _, dep := range deps {
		if addr.Addr == dep.Addr {
			if addr.Txs[0].BlockHeight == -1 {
				addr.Txs[0] = dep.Tx
			} else {
				if !ExistTx(addr, dep.Tx) {
					addr.Txs = append(addr.Txs, dep.Tx)
				}
			}
		}
	}

	return addr
}

func ExistTx(addr Address, tx Tx) bool {
	for _, t := range addr.Txs {
		if t == tx {
			return true
		}
	}
	return false
}

func UpdateAddressInfo(addrs []Address, deps []Deposit, blockID int64) []Address {
	branch := 0
	for i, addr := range addrs {
		branch = 0

		if addr.MaxScanBlock == 0 && branch == 0 && blockID > 1 {
			addr = CompareAddress(addr, deps)
			addrs[i].Txs = addr.Txs
			addrs[i].MaxScanBlock = blockID
			addrs[i].MidScanBlock = blockID
			branch = 1
		}

		if addr.MinScanBlock < addr.MaxScanBlock && addr.MinScanBlock == (blockID-1) && branch == 0 {
			addr = CompareAddress(addr, deps)
			addrs[i].Txs = addr.Txs
			addrs[i].MinScanBlock = blockID
			if addrs[i].MinScanBlock > addrs[i].MidScanBlock {
				addrs[i].MidScanBlock = addrs[i].MinScanBlock
			}
			branch = 1
		}

		if addr.MaxScanBlock > addr.MinScanBlock && addr.MaxScanBlock == (blockID-1) && branch == 0 {
			addr = CompareAddress(addr, deps)
			addrs[i].Txs = addr.Txs
			addrs[i].MaxScanBlock = blockID
			branch = 1
		}

		if addr.MinScanBlock == addr.MidScanBlock && addr.MinScanBlock == addr.MaxScanBlock && addr.MaxScanBlock == (blockID-1) && branch == 0 {
			addr = CompareAddress(addr, deps)
			addrs[i].Txs = addr.Txs
			addrs[i].MaxScanBlock = blockID
			addrs[i].MidScanBlock = blockID
			addrs[i].MinScanBlock = blockID
			branch = 1
		}

	}
	return addrs
}

func LoadWallet(file string) ([]Address, error) {
	var addrs []Address
	wallet, err := os.Open(file)
	defer wallet.Close()
	if err != nil {
		return nil, err
	}
	jsonParser := json.NewDecoder(wallet)
	jsonParser.Decode(&addrs)
	return addrs, nil
}

func SaveWallet(file string, addrs []Address) error {
	wallet, err := os.Open(file)
	defer wallet.Close()
	if err != nil {
		return err
	}
	addrsJson, _ := json.MarshalIndent(addrs, "", "    ")
	err = ioutil.WriteFile(file, addrsJson, 0644)
	return nil
}

func NewBTCDClient(username, pass string) (*rpcclient.Client, error) {
	//find path to btcd
	btcdHomeDir := btcutil.AppDataDir("btcd", false)
	certs, err := ioutil.ReadFile(filepath.Join(btcdHomeDir, "rpc.cert"))
	if err != nil {
		return nil, err
	}
	//config settings
	connCfg := &rpcclient.ConnConfig{
		Host:         "localhost:8334",
		Endpoint:     "ws",
		User:         username,
		Pass:         pass,
		Certificates: certs,
	}
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func main() {
	//btcd settings
	btcdUser := "myuser"
	btcdPass := "SomeDecentp4ssw0rd"

	//flags
	wallet := flag.String("wallet", "wallet.json", "wallet.json file")
	blockN := flag.Int64("n", 0, "btc_addresses.json file")
	blockM := flag.Int64("m", 0, "debug mode will show more detail logs")
	flag.Parse()

	//flags validation
	if *blockN < 0 || *blockM < 0 || *blockM < *blockN {
		fmt.Println("Bad block range")
		return
	}

	addrs, err := LoadWallet(*wallet)
	if err != nil {
		fmt.Println("Wallet loading is failed:", err)
		return
	}

	//create btcd instance
	client, err := NewBTCDClient(btcdUser, btcdPass)
	defer client.Shutdown()

	for i := int(*blockN); i <= int(*blockM); i++ {
		//fmt.Println("Scannig block: ", i)
		deposits, err := ScanBlock(client, int64(i))
		if err != nil {
			fmt.Println("Block scanning is failed:", err)
			return
		}

		addrs = UpdateAddressInfo(addrs, deposits, int64(i))

	}

	SaveWallet(*wallet, addrs)

}
