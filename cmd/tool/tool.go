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

	"github.com/boltdb/bolt"
)

// lastScanBlock struct in bucket
type lastScanBlock struct {
	Hash   string
	Height int64
}

// btc address json struct
type addressJSON struct {
	BtcAddresses []string `json:"btc_addresses"`
}

var (
	scanMetaBkt      = []byte("scan_meta")
	lastScanBlockKey = []byte("last_scan_block")
)

func main() {
	u, _ := user.Current()
	dbFile := flag.String("db", filepath.Join(u.HomeDir, ".skycoin-teller/data.db"), "db file path")
	btcAddrFile := flag.String("btcfile", "../teller/btc_addresses.json", "btc addresses json file")

	flag.Parse()

	if _, err := os.Stat(*dbFile); os.IsNotExist(err) {
		fmt.Println(*dbFile, "does not exist")
		return
	}

	db, err := bolt.Open(*dbFile, 0700, nil)
	if err != nil {
		log.Printf("Open db failed: %v\n", err)
		return
	}

	defer db.Close()

	args := flag.Args()

	if len(args) > 0 {
		cmd := args[0]
		switch cmd {
		case "setlastscanblock":
			height, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				fmt.Println("Invalid argument for set_lastscan_block command")
				return
			}

			hash := args[2]
			lb := lastScanBlock{
				Height: height,
				Hash:   hash,
			}

			db.Update(func(tx *bolt.Tx) error {
				bkt := tx.Bucket(scanMetaBkt)
				v, err := json.Marshal(lb)
				if err != nil {
					fmt.Println("Marshal lastscanblock failed:", err)
					return nil
				}

				return bkt.Put(lastScanBlockKey, v)
			})
		case "getlastscanblock":
			db.View(func(tx *bolt.Tx) error {
				v := tx.Bucket(scanMetaBkt).Get(lastScanBlockKey)
				var lsb lastScanBlock
				if err := json.Unmarshal(v, &lsb); err != nil {
					fmt.Println("Unmarshal failed:", err)
					return nil
				}

				v, err := json.MarshalIndent(lsb, "", "    ")
				if err != nil {
					fmt.Println("MarshalIndent failed:", err)
					return nil
				}

				fmt.Println(string(v))
				return nil
			})
		case "addbtcaddress":
			v, err := ioutil.ReadFile(*btcAddrFile)
			if err != nil {
				fmt.Println("Read btcjson file failed:", err)
				return
			}

			var addrJSON addressJSON
			if err := json.NewDecoder(bytes.NewReader(v)).Decode(&addrJSON); err != nil {
				fmt.Println("Decode btcaddr json failed:", err)
				return
			}

			addrJSON.BtcAddresses = append(addrJSON.BtcAddresses, args[1])
			v, err = json.MarshalIndent(addrJSON, "", "    ")
			if err != nil {
				fmt.Println("MarshalIndent btc addresses failed:", err)
				return
			}
			ioutil.WriteFile(*btcAddrFile, v, 0700)
		case "getbtcaddress":
			v, err := ioutil.ReadFile(*btcAddrFile)
			if err != nil {
				fmt.Println("Read btcjson file failed:", err)
				return
			}
			fmt.Println(string(v))
		default:
			log.Printf("Unknow command: %s\n", cmd)
		}
	}
}
