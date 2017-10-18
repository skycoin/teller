package addrs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"

	"github.com/skycoin/skycoin/src/cipher"
)

const btcBucketKeyUsed = "used_btc_address"
const btcBucketKeyAll = "all_btc_address"

// NewBTCAddrs returns an Addrs loaded with BTC addresses
func NewBTCAddrs(log logrus.FieldLogger, db *bolt.DB, addrsReader io.Reader) (*Addrs, error) {
	loader, err := loadBTCAddresses(addrsReader)
	if err != nil {
		return nil, err
	}
	return NewAddrs(log, db, loader, btcBucketKeyUsed, btcBucketKeyAll)
}

func GetBTCManager(log logrus.FieldLogger, db *bolt.DB) (*Addrs, error) {
	manager, err := GetAddressManager(log, db, btcBucketKeyUsed, btcBucketKeyAll)
	if err != nil {
		return nil, err
	}
	return manager, nil
}

func loadBTCAddresses(addrsReader io.Reader) ([]string, error) {
	var addrs struct {
		Addresses []string `json:"btc_addresses"`
	}

	if err := json.NewDecoder(addrsReader).Decode(&addrs); err != nil {
		return nil, fmt.Errorf("Decode loaded address json failed: %v", err)
	}

	if err := verifyBTCAddresses(addrs.Addresses); err != nil {
		return nil, err
	}

	return addrs.Addresses, nil
}

func verifyBTCAddresses(addrs []string) error {
	if len(addrs) == 0 {
		return errors.New("No BTC addresses")
	}

	addrMap := make(map[string]struct{}, len(addrs))

	for _, addr := range addrs {
		if _, ok := addrMap[addr]; ok {
			return fmt.Errorf("Duplicate deposit address `%s`", addr)
		}

		if _, err := cipher.BitcoinDecodeBase58Address(addr); err != nil {
			return fmt.Errorf("Invalid deposit address `%s`: %v", addr, err)
		}

		addrMap[addr] = struct{}{}
	}

	return nil
}
