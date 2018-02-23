package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/boltdb/bolt"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/teller/src/exchange"
	"github.com/skycoin/teller/src/scanner"
)

const usage = `dbfix <dbfile>

Applies fixes to bad data in the db
`

func run(dbName string) error {
	db, err := bolt.Open(dbName, 0700, nil)
	if err != nil {
		return err
	}

	return fixSkyAddrWhitespace(db)
}

func fixSkyAddrWhitespace(db *bolt.DB) error {
	// Invalid skycoin addresses were saved to the db at one point,
	// due to a bug in base58 address verification.
	// Any non-base58 (bitcoin alphabet) character was treated as the
	// character "1" (equal to 0 in this base58 alphabet).
	// Notably, this allowed leading whitespace in addresses, since base58
	// allows indefinite leading zeroes (i.e. leading "1" characters).

	// Re-verify skycoin addresses saved in the database, and if possible
	// repair them.

	// At the time of this writing, there were a few skycoin addresses like this,
	// and all had a single leading whitespace character.

	// Read various buckets that have sky address
	// Trim whitespace, save entry

	// From exchange:
	// SkyDepositSeqsIndexBkt
	// BindAddressBkt
	// DepositInfoBkt

	if err := fixSkyDepositSeqsIndexBkt(db); err != nil {
		return err
	}

	if err := fixBindAddressBkt(db); err != nil {
		return err
	}

	return fixDepositInfoBkt(db)
}

func fixAddress(a string) (string, error) {
	fixedAddr := strings.Trim(a, "\n\t ")
	if _, err := cipher.DecodeBase58Address(fixedAddr); err != nil {
		return "", fmt.Errorf("Skycoin address \"%s\" changed to \"%s\" but still invalid: %v", a, fixedAddr, err)
	}

	if fixedAddr == a {
		return "", errors.New("address does not need fixing")
	}

	return fixedAddr, nil
}

func fixBindAddressBkt(db *bolt.DB) error {
	// Check BindAddressBkt
	// It maps a btc address to a sky address, so check the value

	var invalidBtcAddrs [][]byte
	if err := db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(exchange.MustGetBindAddressBkt(scanner.CoinTypeBTC))
		if bkt == nil {
			return errors.New("BindAddressBkt not found in db")
		}

		return bkt.ForEach(func(k, v []byte) error {
			skyAddr := string(v)
			if _, err := cipher.DecodeBase58Address(skyAddr); err != nil {
				fmt.Printf("Found invalid sky address \"%s\" in BindAddressBkt: %v\n", skyAddr, err)
				invalidBtcAddrs = append(invalidBtcAddrs, k)
			}

			return nil
		})
	}); err != nil {
		return err
	}

	fmt.Printf("Repairing BindAddressBkt, %d invalid addresses\n", len(invalidBtcAddrs))
	return db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(exchange.MustGetBindAddressBkt(scanner.CoinTypeBTC))
		for _, a := range invalidBtcAddrs {
			v := bkt.Get(a)
			if v == nil {
				return fmt.Errorf("value expectedly missing for key \"%s\"", string(a))
			}

			skyAddr := string(v)
			fixedAddr, err := fixAddress(skyAddr)
			if err != nil {
				return err
			}

			if err := bkt.Put(a, []byte(fixedAddr)); err != nil {
				return err
			}
		}

		return nil
	})
}

func fixDepositInfoBkt(db *bolt.DB) error {
	// Check DepositInfoBkt
	// It maps a BTC transaction to a DepositInfo
	// The invalid sky address is attached to the DepositInfo

	var invalidBtcTxs [][]byte
	if err := db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(exchange.DepositInfoBkt)
		if bkt == nil {
			return errors.New("DepositInfoBkt not found in db")
		}

		return bkt.ForEach(func(k, v []byte) error {
			var dpi exchange.DepositInfo
			if err := json.Unmarshal(v, &dpi); err != nil {
				return err
			}

			if _, err := cipher.DecodeBase58Address(dpi.SkyAddress); err != nil {
				fmt.Printf("Found invalid sky address \"%s\" in DepositInfoBkt: %v\n", dpi.SkyAddress, err)
				invalidBtcTxs = append(invalidBtcTxs, k)
			}

			return nil
		})
	}); err != nil {
		return err
	}

	fmt.Printf("Repairing DepositInfoBkt, %d invalid addresses\n", len(invalidBtcTxs))
	return db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(exchange.DepositInfoBkt)
		for _, a := range invalidBtcTxs {
			v := bkt.Get(a)
			if v == nil {
				return fmt.Errorf("value unexpectedly missing for key \"%s\"", string(a))
			}

			var dpi exchange.DepositInfo
			if err := json.Unmarshal(v, &dpi); err != nil {
				return err
			}

			fixedAddr, err := fixAddress(dpi.SkyAddress)
			if err != nil {
				return err
			}

			dpi.SkyAddress = fixedAddr

			b, err := json.Marshal(&dpi)
			if err != nil {
				return err
			}

			if err := bkt.Put(a, b); err != nil {
				return err
			}
		}

		return nil
	})
}

func fixSkyDepositSeqsIndexBkt(db *bolt.DB) error {
	// Check SkyDepositSeqsIndexBkt
	// It maps a sky address to btc addresses, so check the key

	var invalidSkyAddrs [][]byte
	if err := db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(exchange.SkyDepositSeqsIndexBkt)
		if bkt == nil {
			return errors.New("SkyDepositSeqsIndexBkt not found in db")
		}

		return bkt.ForEach(func(k, v []byte) error {
			skyAddr := string(k)
			if _, err := cipher.DecodeBase58Address(skyAddr); err != nil {
				fmt.Printf("Found invalid sky address \"%s\" in SkyDepositSeqsIndexBkt: %v\n", skyAddr, err)
				invalidSkyAddrs = append(invalidSkyAddrs, k)
			}

			return nil
		})
	}); err != nil {
		return err
	}

	fmt.Printf("Repairing SkyDepositSeqsIndexBkt, %d invalid addresses\n", len(invalidSkyAddrs))
	return db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(exchange.SkyDepositSeqsIndexBkt)
		for _, a := range invalidSkyAddrs {

			v := bkt.Get(a)
			if v == nil {
				return fmt.Errorf("value unexpectedly missing for key \"%s\"", string(a))
			}

			fixedAddr, err := fixAddress(string(a))
			if err != nil {
				return err
			}

			if err := bkt.Put([]byte(fixedAddr), v); err != nil {
				return err
			}

			if err := bkt.Delete(a); err != nil {
				return err
			}
		}

		return nil
	})
}

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Println(usage)
		os.Exit(1)
	}

	dbName := flag.Arg(0)

	if err := run(dbName); err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
}
