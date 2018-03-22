package addrs

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
	"github.com/MDLlife/teller/src/util"
)

const ethBucketKey = "used_eth_address"

// NewETHAddrs returns an Addrs loaded with ETH addresses
func NewETHAddrs(log logrus.FieldLogger, db *bolt.DB, addrsReader io.Reader) (*Addrs, error) {
	loader, err := loadETHAddresses(addrsReader)
	if err != nil {
		log.WithError(err).Error("Load deposit ethereum address list failed")
		return nil, err
	}
	return NewAddrs(log, db, loader, ethBucketKey)
}

func loadETHAddresses(addrsReader io.Reader) (addrs []string, err error) {
	addrs, err = util.ReadLines(addrsReader)
	if err != nil {
		return nil, fmt.Errorf("Decode loaded address failed: %v", err)
	}

	if err := verifyETHAddresses(addrs); err != nil {
		return nil, err
	}

	return addrs, nil
}

// https://github.com/ethereum/go-ethereum/blob/2db97986460c57ba74a563d97a704a45a270df7d/common/icap.go
func validCheckSum(s string) error {
	if len(s) != 42 {
		return errors.New("Invalid address length")
	}
	if strings.HasPrefix(s, "0x") {
		return nil
	}
	return errors.New("invalid address")
}

func verifyETHAddresses(addrs []string) error {
	if len(addrs) == 0 {
		return errors.New("No ETH addresses")
	}

	addrMap := make(map[string]struct{}, len(addrs))

	for _, addr := range addrs {
		if _, ok := addrMap[addr]; ok {
			return fmt.Errorf("Duplicate deposit address `%s`", addr)
		}

		if err := validCheckSum(addr); err != nil {
			return fmt.Errorf("Invalid deposit address `%s`: %v", addr, err)
		}

		addrMap[addr] = struct{}{}
	}

	return nil
}
