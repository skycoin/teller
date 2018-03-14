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

const skyBucketKey = "used_sky_address"

func NewSKYAddrs(log logrus.FieldLogger, db *bolt.DB, addrsReader io.Reader) (*Addrs, error) {
	loader, err := loadSKYAddresses(addrsReader)
	if err != nil {
		return nil, err
	}
	return NewAddrs(log, db, loader, skyBucketKey)
}

func loadSKYAddresses(addrsReader io.Reader) ([]string, error) {
	var addrs struct {
		Addresses []string `json:"sky_addresses"`
	}

	if err := json.NewDecoder(addrsReader).Decode(&addrs); err != nil {
		return nil, fmt.Errorf("Decode loaded address json failed: %v", err)
	}

	if err := verifySKYAddresses(addrs.Addresses); err != nil {
		return nil, err
	}

	return addrs.Addresses, nil
}

func verifySKYAddresses(addrs []string) error {
	if len(addrs) == 0 {
		return errors.New("No SKY addresses")
	}

	addrMap := make(map[string]struct{}, len(addrs))

	for _, addr := range addrs {
		if _, ok := addrMap[addr]; ok {
			return fmt.Errorf("Duplicate deposit address `%s`", addr)
		}

		if _, err := cipher.DecodeBase58Address(addr); err != nil {
			return fmt.Errorf("Invalid deposit address `%s`: %v", addr, err)
		}

		addrMap[addr] = struct{}{}
	}

	return nil
}
