package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"errors"
	"strings"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcutil"
)

type NewBTCAddress struct {
	Addresses []string `json:"btc_addresses"`
}

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
			depTx.TxHash = fmt.Sprintf("%s:%d", tx.Txid, i)

			for _, newAddr := range addr.ScriptPubKey.Addresses {
				deposits = append(deposits, Deposit{
					Addr: newAddr,
					Tx:   depTx,
				})
			}
		}

	}
	return deposits, nil
}

func FindTxs(addr Address, deps []Deposit) []Tx {
	var txs []Tx
	for _, dep := range deps {
		if addr.Addr == dep.Addr && !ExistTx(addr, dep.Tx) {
			txs = append(txs, dep.Tx)
		}
	}

	return txs
}

func ExistTx(addr Address, tx Tx) bool {
	for _, t := range addr.Txs {
		if t == tx {
			return true
		}
	}
	return false
}

func ExistAddress(newAddr Address, walletAddresses []Address) bool {
	for _, addr := range walletAddresses {
		if newAddr.Addr == addr.Addr {
			return true
		}
	}
	return false
}

func UpdateAddressInfo(addrs []Address, deps []Deposit, blockID int64) []Address {
	for i, addr := range addrs {
		switch {
		case addr.MaxScanBlock == 0 && blockID > 1:
			txs := FindTxs(addr, deps)
			addrs[i].Txs = append(addr.Txs, txs...)
			addrs[i].MaxScanBlock = blockID
			addrs[i].MidScanBlock = blockID
		case addr.MinScanBlock < addr.MaxScanBlock && addr.MinScanBlock == (blockID-1):
			txs := FindTxs(addr, deps)
			addrs[i].Txs = append(addr.Txs, txs...)
			addrs[i].MinScanBlock = blockID
			if addrs[i].MinScanBlock > addrs[i].MidScanBlock {
				addrs[i].MidScanBlock = addrs[i].MinScanBlock
			}
		case addr.MaxScanBlock > addr.MinScanBlock && addr.MaxScanBlock == (blockID-1):
			txs := FindTxs(addr, deps)
			addrs[i].Txs = append(addr.Txs, txs...)
			addrs[i].MaxScanBlock = blockID
		case addr.MinScanBlock == addr.MidScanBlock && addr.MinScanBlock == addr.MaxScanBlock && addr.MaxScanBlock == (blockID-1):
			txs := FindTxs(addr, deps)
			addrs[i].Txs = append(addr.Txs, txs...)
			addrs[i].MaxScanBlock = blockID
			addrs[i].MidScanBlock = blockID
			addrs[i].MinScanBlock = blockID
		}

	}
	return addrs
}

func LoadWallet(file string) ([]Address, error) {
	var addrs []Address
	wallet, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	jsonParser := json.NewDecoder(wallet)
	err = jsonParser.Decode(&addrs)
	if err != nil {
		return nil, err
	}
	return addrs, wallet.Close()
}

func SaveWallet(file string, addrs []Address) error {
	wallet, err := os.Open(file)
	if err != nil {
		return err
	}
	addrsJson, err := json.MarshalIndent(addrs, "", "    ")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(file, addrsJson, 0644)
	if err != nil {
		return err
	}
	return wallet.Close()
}

func LoadBTCFromFile(file string) (NewBTCAddress, error) {
	var addrs NewBTCAddress
	userFile, err := os.Open(file)
	if err != nil {
		return addrs, err
	}
	jsonParser := json.NewDecoder(userFile)
	err = jsonParser.Decode(&addrs)
	if err != nil {
		return addrs, err
	}
	return addrs, userFile.Close()
}

func AddBTCAddress(addr string, file string) error {
	newAddr := Address{
		Addr:         addr,
		MinScanBlock: 0,
		MidScanBlock: 0,
		MaxScanBlock: 0,
		Txs:          []Tx{},
	}

	addrs, err := LoadWallet(file)
	if err != nil {
		return err
	}

	if !ExistAddress(newAddr, addrs) {
		addrs = append(addrs, newAddr)
	}

	err = SaveWallet(file, addrs)
	if err != nil {
		return err
	}

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

func run() error {
	//flags
	user := flag.String("user", "myuser", "btcd username")
	pass := flag.String("pass", "SomeDecentp4ssw0rd", "btcd password")
	wallet := flag.String("wallet", "wallet.json", "wallet.json file")
	blockN := flag.Int64("n", 0, "start blockID")
	blockM := flag.Int64("m", 0, "finish blockID")
	add := flag.String("add", "", "new btc addresses")
	addFile := flag.String("add_file", "", "new btc addresses from file")
	flag.Parse()

	if *add != "" {
		newBTCAddrs := strings.Split(*add, ",")
		for _, addr := range newBTCAddrs {
			AddBTCAddress(addr, *wallet)
		}
		fmt.Println("Addresses from command line added.")
	}

	if *addFile != "" {
		newBTC, _ := LoadBTCFromFile(*addFile)
		for _, addr := range newBTC.Addresses {
			AddBTCAddress(addr, *wallet)
		}
		fmt.Println("Addresses from file added.")
	}
	//flags validation
	if *blockN < 0 || *blockM < 0 || *blockM < *blockN {
		return errors.New("Bad block range")
	}

	addrs, err := LoadWallet(*wallet)
	if err != nil {
		fmt.Println("Wallet loading is failed:", err)
		return err
	}

	//create btcd instance
	client, err := NewBTCDClient(*user, *pass)
	defer client.Shutdown()

	for i := int(*blockN); i <= int(*blockM); i++ {
		//fmt.Println("Scannig block: ", i)
		deposits, err := ScanBlock(client, int64(i))
		if err != nil {
			fmt.Println("Block scanning is failed:", err)
			return err
		}

		addrs = UpdateAddressInfo(addrs, deposits, int64(i))

	}

	err = SaveWallet(*wallet, addrs)
	if err != nil {
		fmt.Println("Saving wallet is failed:", err)
		return err
	}
	return nil
}

func main() {

	if err := run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

}
