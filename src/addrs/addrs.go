// Package addrs manages deposit bitcoin addresses
package addrs

import (
	"errors"
	"fmt"
	"sync"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
)

// ErrDepositAddressEmpty represents all deposit addresses are used
var ErrDepositAddressEmpty = errors.New("Deposit address pool is empty")

// AddrGenerator generate new deposit address
type AddrGenerator interface {
	NewAddress() (string, error)
}

// Addrs manages deposit addresses
type Addrs struct {
	sync.RWMutex
	log       logrus.FieldLogger
	used      *Store   // all used addresses
	all       *Store   // all addresses
	addresses []string // address pool for deposit
}

//GetAddressManager create instance for manage addresses
func GetAddressManager(log logrus.FieldLogger, db *bolt.DB, bucketKeyUsed string, bucketKeyAll string) (*Addrs, error) {
	used, err := NewStore(db, bucketKeyUsed)
	if err != nil {
		return nil, err
	}
	all, err := NewStore(db, bucketKeyAll)
	if err != nil {
		return nil, err
	}

	addresses, err := all.GetAll()
	if err != nil {
		return nil, err
	}

	return &Addrs{
		log:       log.WithField("prefix", "addrs"),
		used:      used,
		all:       all,
		addresses: addresses,
	}, nil
}

// NewAddrs creates Addrs instance, will load and verify the addresses
func NewAddrs(log logrus.FieldLogger, db *bolt.DB, addresses []string, bucketKeyUsed string, bucketKeyAll string) (*Addrs, error) {
	used, err := NewStore(db, bucketKeyUsed)
	if err != nil {
		return nil, err
	}

	addresses, err = removeUsedAddresses(used, addresses)
	if err != nil {
		return nil, err
	}

	all, err := NewStore(db, bucketKeyAll)
	if err != nil {
		return nil, err
	}

	for _, addr := range addresses {
		exist, err := all.IsUsed(addr)
		if err != nil {
			return nil, fmt.Errorf("Checking exist failed: %v", err)
		}
		fmt.Println(exist)
		if !exist {
			if err = all.Put(addr); err != nil {
				return nil, fmt.Errorf("Put address in all pool failed: %v", err)
			}
		}
	}

	return &Addrs{
		log:       log.WithField("prefix", "addrs"),
		used:      used,
		all:       all,
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

func (a *Addrs) GetAll() ([]string, error) {
	a.RLock()
	defer a.RUnlock()

	return a.all.GetAll()
}

func (a *Addrs) GetUsed() ([]string, error) {
	a.RLock()
	defer a.RUnlock()

	return a.used.GetAll()
}

func (a *Addrs) SetUsed(addr string) error {
	a.RLock()
	defer a.RUnlock()

	err := a.used.Put(addr)
	if err != nil {
		return err
	}

	return nil
}

func (a *Addrs) GetFree() ([]string, error) {
	a.RLock()
	defer a.RUnlock()
	var free []string

	all, err := a.all.GetAll()
	if err != nil {
		return nil, err
	}

	for _, addr := range all {
		exist, err := a.used.IsUsed(addr)
		if err != nil {
			return nil, err
		}

		if !exist {
			free = append(free, addr)
		}
	}

	return free, nil
}

// Remaining returns the rest btc address number
func (a *Addrs) Remaining() uint64 {
	a.RLock()
	defer a.RUnlock()

	return uint64(len(a.addresses))
}
