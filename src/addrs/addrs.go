// Package addrs manages deposit bitcoin addresses
package addrs

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"sync"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
)

var (
	// ErrDepositAddressEmpty represents all deposit addresses are used
	ErrDepositAddressEmpty = errors.New("Deposit address pool is empty")
	// ErrCoinTypeNotRegistered is returned when an unrecognized coin type is used
	ErrCoinTypeNotRegistered = errors.New("Coin type is not registered")
)

const (
	jsonExtension = ".json"
)

// AddrGenerator generate new deposit address
type AddrGenerator interface {
	NewAddress() (string, error)
	Remaining() uint64
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
	sync.RWMutex
	generators map[string]AddrGenerator
}

// NewAddrManager create a Manager
func NewAddrManager() *AddrManager {
	return &AddrManager{
		generators: make(map[string]AddrGenerator),
	}
}

// PushGenerator add a AddrGenerater with coinType
func (am *AddrManager) PushGenerator(ag AddrGenerator, coinType string) error {
	am.Lock()
	defer am.Unlock()

	if _, ok := am.generators[coinType]; ok {
		return errors.New("coinType already exists")
	}

	am.generators[coinType] = ag

	return nil
}

// NewAddress return new address according to coinType
func (am *AddrManager) NewAddress(coinType string) (string, error) {
	am.Lock()
	defer am.Unlock()

	ag, ok := am.generators[coinType]
	if !ok {
		return "", ErrCoinTypeNotRegistered
	}

	return ag.NewAddress()
}

// Remaining returns the number of remaining addresses for a given coin type
func (am *AddrManager) Remaining(coinType string) (uint64, error) {
	am.Lock()
	defer am.Unlock()

	ag, ok := am.generators[coinType]
	if !ok {
		return 0, ErrCoinTypeNotRegistered
	}

	return ag.Remaining(), nil
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

// loadAddresses parses a newline-separated list of addresses, skipping empty lines
// and lines starting with #
func loadAddresses(addrsReader io.Reader) ([]string, error) {
	all, err := ioutil.ReadAll(addrsReader)
	if err != nil {
		return nil, fmt.Errorf("Failed to read addresses file: %v", err)
	}

	// Filter empty lines and comments
	addrs := strings.Split(string(all), "\n")
	for i := 0; i < len(addrs); i++ {
		a := strings.TrimSpace(addrs[i])
		if a == "" || a[0] == '#' {
			addrs = append(addrs[:i], addrs[i+1:]...)
			i--
		}
	}

	return addrs, nil
}
