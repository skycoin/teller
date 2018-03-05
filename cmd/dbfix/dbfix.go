package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/boltdb/bolt"

	"github.com/MDLlife/MDL/src/cipher"

	"github.com/MDLlife/teller/src/exchange"
	"github.com/MDLlife/teller/src/scanner"
)

const usage = `dbfix <dbfile>

Applies fixes to bad data in the db
`

func run(dbName string) error {
	db, err := bolt.Open(dbName, 0700, nil)
	if err != nil {
		return err
	}

	return fixMDLAddrWhitespace(db)
}

func fixMDLAddrWhitespace(db *bolt.DB) error {
	// Invalid mdl addresses were saved to the db at one point,
	// due to a bug in base58 address verification.
	// Any non-base58 (bitcoin alphabet) character was treated as the
	// character "1" (equal to 0 in this base58 alphabet).
	// Notably, this allowed leading whitespace in addresses, since base58
	// allows indefinite leading zeroes (i.e. leading "1" characters).

	// Re-verify mdl addresses saved in the database, and if possible
	// repair them.

	// At the time of this writing, there were a few mdl addresses like this,
	// and all had a single leading whitespace character.

	// Read various buckets that have mdl address
	// Trim whitespace, save entry

	// From exchange:
	// MDLDepositSeqsIndexBkt
	// BindAddressBkt
	// DepositInfoBkt

	if err := fixMDLDepositSeqsIndexBkt(db); err != nil {
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
		return "", fmt.Errorf("MDL address \"%s\" changed to \"%s\" but still invalid: %v", a, fixedAddr, err)
	}

	if fixedAddr == a {
		return "", errors.New("address does not need fixing")
	}

	return fixedAddr, nil
}

func fixBindAddressBkt(db *bolt.DB) error {
	// Check BindAddressBkt
	// It maps a btc address to a mdl address, so check the value

	var invalidBtcAddrs [][]byte
	if err := db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(exchange.MustGetBindAddressBkt(scanner.CoinTypeBTC))
		if bkt == nil {
			return errors.New("BindAddressBkt not found in db")
		}

		return bkt.ForEach(func(k, v []byte) error {
			mdlAddr := string(v)
			if _, err := cipher.DecodeBase58Address(mdlAddr); err != nil {
				fmt.Printf("Found invalid mdl address \"%s\" in BindAddressBkt: %v\n", mdlAddr, err)
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

			mdlAddr := string(v)
			fixedAddr, err := fixAddress(mdlAddr)
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
	// The invalid mdl address is attached to the DepositInfo

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

			if _, err := cipher.DecodeBase58Address(dpi.MDLAddress); err != nil {
				fmt.Printf("Found invalid mdl address \"%s\" in DepositInfoBkt: %v\n", dpi.MDLAddress, err)
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

			fixedAddr, err := fixAddress(dpi.MDLAddress)
			if err != nil {
				return err
			}

			dpi.MDLAddress = fixedAddr

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

func fixMDLDepositSeqsIndexBkt(db *bolt.DB) error {
	// Check MDLDepositSeqsIndexBkt
	// It maps a mdl address to btc addresses, so check the key

	var invalidMDLAddrs [][]byte
	if err := db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(exchange.MDLDepositSeqsIndexBkt)
		if bkt == nil {
			return errors.New("MDLDepositSeqsIndexBkt not found in db")
		}

		return bkt.ForEach(func(k, v []byte) error {
			mdlAddr := string(k)
			if _, err := cipher.DecodeBase58Address(mdlAddr); err != nil {
				fmt.Printf("Found invalid mdl address \"%s\" in MDLDepositSeqsIndexBkt: %v\n", mdlAddr, err)
				invalidMDLAddrs = append(invalidMDLAddrs, k)
			}

			return nil
		})
	}); err != nil {
		return err
	}

	fmt.Printf("Repairing MDLDepositSeqsIndexBkt, %d invalid addresses\n", len(invalidMDLAddrs))
	return db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(exchange.MDLDepositSeqsIndexBkt)
		for _, a := range invalidMDLAddrs {

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
