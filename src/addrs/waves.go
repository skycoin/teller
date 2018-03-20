package addrs

import (
	"encoding/json"
	"fmt"
	"io"

	"errors"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
	"github.com/skycoin/skycoin/src/cipher"
)

const wavesBucketKey = "used_waves_address"

// NewWAVESAddrs returns an Addrs loaded with WAVES addresses
func NewWAVESAddrs(log logrus.FieldLogger, db *bolt.DB, addrsReader io.Reader) (*Addrs, error) {
	loader, err := loadWAVESAddresses(addrsReader)
	if err != nil {
		return nil, err
	}
	return NewAddrs(log, db, loader, wavesBucketKey)
}

func loadWAVESAddresses(addrsReader io.Reader) ([]string, error) {
	var addrs struct {
		Addresses []string `json:"waves_addresses"`
	}

	if err := json.NewDecoder(addrsReader).Decode(&addrs); err != nil {
		return nil, fmt.Errorf("Decode loaded address json failed: %v", err)
	}

	if err := verifyWAVESAddresses(addrs.Addresses); err != nil {
		return nil, err
	}

	return addrs.Addresses, nil
}

//func validWAVESCheckSum(s string) error {
//	if len(s) != 34 && len(s) != 35 {
//		fmt.Println("validWAVESCheckSum, ", len(s))
//		return errors.New("Invalid address length")
//	}
//	return nil
//}

func verifyWAVESAddresses(addrs []string) error {
	if len(addrs) == 0 {
		return errors.New("No WAVES addresses")
	}

	addrMap := make(map[string]struct{}, len(addrs))

	for _, addr := range addrs {
		if _, ok := addrMap[addr]; ok {
			return fmt.Errorf("Duplicate deposit address `%s`", addr)
		}

		//if err := validWAVESCheckSum(addr); err != nil {
		//	return fmt.Errorf("Invalid deposit address `%s`: %v", addr, err)
		//}

		_, err := cipher.DecodeBase58Address(addr)
		if err != nil {
			return fmt.Errorf("Invalid deposit address `%s`: %v", addr, err)
		}

		addrMap[addr] = struct{}{}
	}

	return nil
}
