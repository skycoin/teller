package main

import (
	"flag"
	"log"
	"os"
	"os/user"

	"encoding/json"
	"fmt"
	"strconv"

	"path/filepath"

	"bytes"
	"io/ioutil"

	"math"

	"github.com/boltdb/bolt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	btcrpcclient "github.com/btcsuite/btcd/rpcclient"

	"github.com/skycoin/skycoin/src/cipher"
)

const (
	scanBlockCmdName = "scanblock"
)

// btc address json struct
type addressJSON struct {
	BtcAddresses []string `json:"btc_addresses"`
}

var usage = fmt.Sprintf(`%s is a teller helper tool:
Usage:
    %s command [arguments]

The commands are:

    addbtcaddress       add the bitcoin address to the deposit address pool
    getbtcaddress       list all bitcoin deposit address in the pool
    newbtcaddress       generate bitcoin address
    scanblock           scan block from specific height to get all vout with interger value
`, filepath.Base(os.Args[0]), filepath.Base(os.Args[0]))

func main() {
	u, err := user.Current()
	if err != nil {
		log.Println("user.Current failed:", err)
		return
	}
	dbFile := flag.String("db", filepath.Join(u.HomeDir, ".teller-skycoin/teller.db"), "db file path")
	btcAddrFile := flag.String("btcfile", "../teller/btc_addresses.json", "btc addresses json file")
	useJSON := flag.Bool("json", false, "Print newbtcaddress output as json")

	flag.Parse()

	args := flag.Args()

	if len(args) == 0 {
		fmt.Println(usage)
		return
	}

	cmd := args[0]

	var db *bolt.DB
	switch cmd {
	case scanBlockCmdName:
		if _, err := os.Stat(*dbFile); os.IsNotExist(err) {
			fmt.Println(*dbFile, "does not exist")
			return
		}

		db, err = bolt.Open(*dbFile, 0700, nil)
		if err != nil {
			log.Println("Open db failed:", err)
			return
		}
		defer func() {
			if err := db.Close(); err != nil {
				log.Println("Close db failed:", err)
			}
		}()
	default:
	}

	switch cmd {
	case "help", "h":
		if len(args) == 1 {
			fmt.Println(usage)
			return
		}

		switch args[1] {
		case "addbtcaddress":
			fmt.Println("usage: addbtcaddress btc_address")
		case "getbtcaddress":
			fmt.Println("usage: getbtcaddress")
		case "newbtcaddress":
			fmt.Println("usage: [-json] newbtcaddress seed num. -json will print as json.")
		case scanBlockCmdName:
			fmt.Println("usage: server user pass cert_path height")
		case "newkeys":
			fmt.Println("usage: newkeys")
		}
		return
	case "newkeys":
		pub, sec := cipher.GenerateKeyPair()
		var keypair = struct {
			Pubkey string
			Seckey string
		}{
			pub.Hex(),
			sec.Hex(),
		}
		v, err := json.MarshalIndent(keypair, "", "    ")
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(string(v))
	case "addbtcaddress":
		v, err := ioutil.ReadFile(*btcAddrFile)
		if err != nil {
			fmt.Println("Read btcAddr json file failed:", err)
			return
		}

		var addrJSON addressJSON
		if err := json.NewDecoder(bytes.NewReader(v)).Decode(&addrJSON); err != nil {
			fmt.Println("Decode btcAddr json file failed:", err)
			return
		}

		addrJSON.BtcAddresses = append(addrJSON.BtcAddresses, args[1])
		v, err = json.MarshalIndent(addrJSON, "", "    ")
		if err != nil {
			fmt.Println("MarshalIndent btc addresses failed:", err)
			return
		}
		if err := ioutil.WriteFile(*btcAddrFile, v, 0700); err != nil {
			fmt.Println("ioutil.WriteFile failed:", err)
			return
		}
	case "getbtcaddress":
		v, err := ioutil.ReadFile(*btcAddrFile)
		if err != nil {
			fmt.Println("Read btcAddr json file failed:", err)
			return
		}
		fmt.Println(string(v))
	case "newbtcaddress":
		count := args[2]
		var n int
		var err error
		if count != "" {
			n, err = strconv.Atoi(count)
			if err != nil {
				fmt.Println("Invalid argument: ", err)
				return
			}
		} else {
			n = 1
		}

		seckeys := cipher.GenerateDeterministicKeyPairs([]byte(args[1]), n)

		var addrs []string
		for _, sec := range seckeys {
			addr := cipher.BitcoinAddressFromPubkey(cipher.PubKeyFromSecKey(sec))
			addrs = append(addrs, addr)
		}

		if *useJSON {
			s := addressJSON{
				BtcAddresses: addrs,
			}

			b, err := json.MarshalIndent(&s, "", "    ")
			if err != nil {
				log.Fatal(b)
				return
			}

			fmt.Println(string(b))

		} else {
			for _, addr := range addrs {
				fmt.Println(addr)
			}
		}

		return
	case scanBlockCmdName:
		if len(args) != 6 {
			fmt.Println("Invalid arguments")
			fmt.Println(usage)
			return
		}
		rpcserv := args[1]
		rpcuser := args[2]
		rpcpass := args[3]
		rpccert := args[4]
		v, err := ioutil.ReadFile(rpccert)
		if err != nil {
			fmt.Println("Read cert file failed:", err)
			return
		}

		btcrpcli, err := btcrpcclient.New(&btcrpcclient.ConnConfig{
			Host:         rpcserv,
			Endpoint:     "ws",
			User:         rpcuser,
			Pass:         rpcpass,
			Certificates: v,
		}, nil)
		if err != nil {
			fmt.Println("Connect btcd failed:", err)
			return
		}

		h := args[5]
		height, err := strconv.ParseInt(h, 10, 64)
		if err != nil {
			fmt.Println("Invalid block height")
			return
		}

		hash, err := btcrpcli.GetBlockHash(height)
		if err != nil {
			fmt.Println("Get block hash failed:", err)
			return
		}

		for {
			bv, err := btcrpcli.GetBlockVerboseTx(hash)
			if err != nil {
				fmt.Printf("Get block of %s failed: %v\n", hash, err)
				return
			}

			for _, tx := range bv.RawTx {
				for _, o := range tx.Vout {
					if math.Trunc(o.Value) == o.Value {
						fmt.Printf("Height: %v Address: %v Value: %v\n",
							bv.Height, o.ScriptPubKey.Addresses, o.Value)
					}
				}
			}

			if bv.NextHash == "" {
				return
			}

			hash, err = chainhash.NewHashFromStr(bv.NextHash)
			if err != nil {
				fmt.Println("Invalid hash:", bv.NextHash, err)
				return
			}
		}

	default:
		log.Printf("Unknown command: %s\n", cmd)
	}
}
