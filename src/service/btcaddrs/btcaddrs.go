// Package btcaddrs manages deposit bitcoin addreesses
package btcaddrs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/boltdb/bolt"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/util/logger"
)

// ErrDepositAddressEmpty represents all deposit addresses are used
var ErrDepositAddressEmpty = errors.New("deposit address pool is empty")

// BtcAddrs manages deposit bitcoin address
type BtcAddrs struct {
	sync.Mutex
	used      *store   // all used addresses
	addresses []string // address pool for deposit
}

// btc address json struct
type addressJSON struct {
	BtcAddresses []string `json:"btc_addresses"`
}

// New creates BtcAddrs instance, will load and verify the addresses
func New(db *bolt.DB, addrsReader io.Reader, log logger.Logger) (*BtcAddrs, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}

	log.Println("Loading deposit address...")
	var addrs addressJSON
	if err := json.NewDecoder(addrsReader).Decode(&addrs); err != nil {
		return nil, fmt.Errorf("Decode loaded address json failed: %v", err)
	}

	usedAddrs, err := newStore(db)
	if err != nil {
		return nil, err
	}

	btcAddr := &BtcAddrs{
		used: usedAddrs,
	}

	addrMap := make(map[string]struct{}, len(addrs.BtcAddresses))

	// check if the loaded addresses were used.
	for _, addr := range addrs.BtcAddresses {
		// dup check
		if _, ok := addrMap[addr]; ok {
			log.Println("Dup deposit btc address:", addr)
			continue
		}

		// verify the address
		_, err := cipher.BitcoinDecodeBase58Address(addr)
		if err != nil {
			log.Printf("Invalid bitcoin address: %s, err:%v\n", addr, err)
			return nil, err
		}

		if exists, err := usedAddrs.IsExist(addr); err != nil {
			return nil, err
		} else if !exists {
			btcAddr.addresses = append(btcAddr.addresses, addr)
			addrMap[addr] = struct{}{}
		}
	}

	return btcAddr, nil
}

// NewAddress return a new deposit address
func (ba *BtcAddrs) NewAddress() (string, error) {
	ba.Lock()
	defer ba.Unlock()
	if len(ba.addresses) == 0 {
		return "", ErrDepositAddressEmpty
	}

	var addr string
	var pt int
	for i, a := range ba.addresses {
		// check if used
		if exists, err := ba.used.IsExist(a); err != nil {
			return "", err
		} else if exists {
			continue
		}

		pt = i
		addr = a
		break
	}

	if err := ba.used.Put(addr); err != nil {
		return "", fmt.Errorf("Put address in used pool failed: %v", err)
	}

	// remove used addr
	ba.addresses = ba.addresses[pt+1:]
	return addr, nil
}

// RestNum returns the rest btc address number
func (ba *BtcAddrs) RestNum() uint64 {
	ba.Lock()
	defer ba.Unlock()

	return uint64(len(ba.addresses))
}
