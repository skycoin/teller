// Package btcaddrs manages deposit bitcoin addreesses
package btcaddrs

import (
	"errors"
	"io"
	"sync"

	"encoding/json"
	"fmt"

	"github.com/boltdb/bolt"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/logger"
)

// ErrDepositAddressEmpty represents all deposit addresses are used
var ErrDepositAddressEmpty = errors.New("deposit address pool is empty")

// BtcAddrs manages deposit bitcoin address
type BtcAddrs struct {
	sync.Mutex
	used      *store              // all used addresses
	addresses map[string]struct{} // address pool for deposit
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

	var addrs addressJSON
	if err := json.NewDecoder(addrsReader).Decode(&addrs); err != nil {
		return nil, fmt.Errorf("Decode loaded address json failed: %v", err)
	}

	usedAddrs, err := newStore(db)
	if err != nil {
		return nil, err
	}

	btcAddr := &BtcAddrs{
		used:      usedAddrs,
		addresses: make(map[string]struct{}),
	}

	// check if the loaded addresses were used.
	for _, addr := range addrs.BtcAddresses {
		// verify the address
		_, err := cipher.BitcoinDecodeBase58Address(addr)
		if err != nil {
			log.Printf("Invalid bitcoin address: %s, err:%v", addr, err)
			continue
		}

		if !usedAddrs.IsExsit(addr) {
			btcAddr.addresses[addr] = struct{}{}
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
	var remove []string
	for a := range ba.addresses {
		// check if used
		if ba.used.IsExsit(a) {
			remove = append(remove, a)
			continue
		}

		addr = a
		remove = append(remove, a)
		break
	}

	if err := ba.used.Put(addr); err != nil {
		return "", fmt.Errorf("Put address in used pool failed: %v", err)
	}

	// remove used addr
	for _, a := range remove {
		delete(ba.addresses, a)
	}
	return addr, nil
}
