// Package addrs manages deposit bitcoin addresses
package addrs

import (
	"errors"
	"fmt"
	"sync"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
)

var (
	// ErrDepositAddressEmpty represents all deposit addresses are used
	ErrDepositAddressEmpty = errors.New("Deposit address pool is empty")
	// ErrCoinTypeNotExists is returned when an unrecognized coin type is used
	ErrCoinTypeNotExists = errors.New("Invalid coin type")
)

// AddrGenerator generate new deposit address
type AddrGenerator interface {
	NewAddress() (string, error)
}

// Addrs manages deposit addresses
type Addrs struct {
	sync.RWMutex
	log       logrus.FieldLogger
	used      *Store   // all used addresses
	addresses []string // address pool for deposit
}

// AddrManager control all AddrGenerator according to coinType
type AddrManager struct {
	Mutex    sync.RWMutex
	AGHolder map[string]AddrGenerator
	AGcount  int
}

// NewAddrManager create a Manager
func NewAddrManager() *AddrManager {
	return &AddrManager{AGcount: 0, AGHolder: make(map[string]AddrGenerator)}
}

// PushGenerator add a AddrGenerater with coinType
func (am *AddrManager) PushGenerator(ag AddrGenerator, coinType string) error {
	am.Mutex.Lock()
	defer am.Mutex.Unlock()
	_, ok := am.AGHolder[coinType]
	if ok {
		return errors.New("coinType already exists")
	}
	am.AGHolder[coinType] = ag
	am.AGcount++
	return nil
}

// NewAddress return new address according to coinType
func (am *AddrManager) NewAddress(coinType string) (string, error) {
	am.Mutex.Lock()
	defer am.Mutex.Unlock()
	ag, ok := am.AGHolder[coinType]
	if !ok {
		return "", ErrCoinTypeNotExists
	}
	depositAddr, err := ag.NewAddress()
	if err != nil {
		return "", err
	}
	return depositAddr, nil
}

// NewAddrs creates Addrs instance, will load and verify the addresses
func NewAddrs(log logrus.FieldLogger, db *bolt.DB, addresses []string, bucketKey string) (*Addrs, error) {
	used, err := NewStore(db, bucketKey)
	if err != nil {
		return nil, err
	}

	addresses, err = removeUsedAddresses(used, addresses)
	if err != nil {
		return nil, err
	}

	return &Addrs{
		log:       log.WithField("prefix", "addrs"),
		used:      used,
		addresses: addresses,
	}, nil
}

func removeUsedAddresses(s *Store, addrs []string) ([]string, error) {
	var newAddrs []string

	for _, addr := range addrs {
		if used, err := s.IsUsed(addr); err != nil {
			return nil, err
		} else if !used {
			newAddrs = append(newAddrs, addr)
		}
	}

	return newAddrs, nil
}

// NewAddress return a new deposit address
func (a *Addrs) NewAddress() (string, error) {
	a.Lock()
	defer a.Unlock()

	if len(a.addresses) == 0 {
		return "", ErrDepositAddressEmpty
	}

	var chosenAddr string
	var pt int
	for i, addr := range a.addresses {
		if used, err := a.used.IsUsed(addr); err != nil {
			return "", err
		} else if used {
			continue
		}

		pt = i
		chosenAddr = addr
		break
	}

	if chosenAddr == "" {
		return "", ErrDepositAddressEmpty
	}

	if err := a.used.Put(chosenAddr); err != nil {
		return "", fmt.Errorf("Put address in used pool failed: %v", err)
	}

	// remove used addr
	a.addresses = a.addresses[pt+1:]
	return chosenAddr, nil
}

// Remaining returns the rest btc address number
func (a *Addrs) Remaining() uint64 {
	a.RLock()
	defer a.RUnlock()

	return uint64(len(a.addresses))
}
