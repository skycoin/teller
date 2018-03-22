package addrs

import (
	"errors"
	"fmt"
	"io"
	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"

	"github.com/MDLlife/MDL/src/cipher"
	"github.com/MDLlife/teller/src/util"
)

const btcBucketKey = "used_btc_address"

// NewBTCAddrs returns an Addrs loaded with BTC addresses
func NewBTCAddrs(log logrus.FieldLogger, db *bolt.DB, addrsReader io.Reader) (*Addrs, error) {
	loader, err := loadBTCAddresses(addrsReader)
	if err != nil {
		log.WithError(err).Error("Load deposit bitcoin address list failed")
		return nil, err
	}
	return NewAddrs(log, db, loader, btcBucketKey)
}

func loadBTCAddresses(addrsReader io.Reader) (addrs []string, err error) {
	addrs, err = util.ReadLines(addrsReader)
	if err != nil {
		return nil, fmt.Errorf("Decode loaded address failed: %v", err)
	}

	if err := verifyBTCAddresses(addrs); err != nil {
		return nil, err
	}

	return addrs, nil
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
