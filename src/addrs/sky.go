package addrs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"

	"github.com/skycoin/skycoin/src/cipher"
)

const skyBucketKey = "used_sky_address"

// NewSKYAddrs returns an Addrs loaded with SKY addresses
func NewSKYAddrs(log logrus.FieldLogger, db *bolt.DB, addrsFile string) (*Addrs, error) {
	f, err := ioutil.ReadFile(addrsFile)
	if err != nil {
		return nil, fmt.Errorf("Load deposit bitcoin address list failed: %v", err)
	}

	ext := filepath.Ext(addrsFile)

	var addrs []string

	switch ext {
	case jsonExtension:
		addrs, err = loadSKYAddressesJSON(bytes.NewReader(f))
	default:
		addrs, err = loadAddresses(bytes.NewReader(f))
	}

	if err != nil {
		return nil, err
	}

	if err := verifySKYAddresses(addrs); err != nil {
		return nil, err
	}

	return NewAddrs(log, db, addrs, skyBucketKey)
}

func loadSKYAddressesJSON(addrsReader io.Reader) ([]string, error) {
	var addrs struct {
		Addresses []string `json:"sky_addresses"`
	}

	if err := json.NewDecoder(addrsReader).Decode(&addrs); err != nil {
		return nil, fmt.Errorf("Decode loaded address json failed: %v", err)
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
