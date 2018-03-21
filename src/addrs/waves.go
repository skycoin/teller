package addrs

import (
	"encoding/json"
	"fmt"
	"io"

	"errors"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/skycoin/src/cipher/base58"
	"golang.org/x/crypto/blake2b"
	"github.com/ethereum/go-ethereum/crypto"
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
//	if len(s) != 35 {
//		fmt.Println("validWAVESCheckSum, ", len(s))
//		return errors.New("Invalid address length")
//	}
//	return nil
//}

// https://github.com/wavesplatform/Waves/wiki/Data-Structures#address
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

		b, err := base58.Base582Hex(addr)
		if err != nil {
			return fmt.Errorf("Invalid deposit address `%s`: %v", addr, err)
		}
		_, err = addressFromBytes(b)
		if err != nil {
			return fmt.Errorf("Invalid deposit address `%s`: %v", addr, err)
		}

		addrMap[addr] = struct{}{}
	}

	return nil
}

// Returns an address given an Address.Bytes()
func addressFromBytes(b []byte) (addr cipher.Address, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	//1	Version 		(0x01)			Byte	0	1
	//2	Address scheme 	0x54 or 0x57 	Byte	1	1
	//3	Public key hash					Bytes	2	20
	//4	Checksum						Bytes	22	4

	if len(b) != 1+1+20+4 {
		return cipher.Address{}, errors.New("Invalid address length")
	}
	a := cipher.Address{}
	copy(a.Key[0:1], b[0:1])
	a.Version = b[0]
	if a.Version != 0x01 {
		return cipher.Address{}, errors.New("Invalid version")
	}

	chksum := secureHash(b[:22])
	var checksum [4]byte
	copy(checksum[0:4], b[22:26])

	if checksum != chksum {
		return cipher.Address{}, errors.New("Invalid checksum")
	}

	return a, nil
}

//Public key hash is first 20 bytes of SecureHash of public key bytes.
// Checksum is first 4 bytes of SecureHash of version, scheme and hash bytes.
// SecureHash is hash function Keccak256(Blake2b256(data)).
func secureHash(data []byte) cipher.Checksum {
	sum := blake2b.Sum256(data)
	b := crypto.Keccak256(sum[:])
	c := cipher.Checksum{}
	copy(c[:], b[:len(c)])
	return c
}
