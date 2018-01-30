package scanner

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/teller/src/util/httputil"
)

// DummyScanner implements the Scanner interface to provide simulated scanning
type DummyScanner struct {
	addrs     []string
	addrsMap  map[string]struct{}
	deposits  chan DepositNote
	coinTypes map[string]struct{}
	log       logrus.FieldLogger
	sync.RWMutex
}

// NewDummyScanner creates a DummyScanner
func NewDummyScanner(log logrus.FieldLogger) *DummyScanner {
	return &DummyScanner{
		log:       log.WithField("prefix", "scanner.dummy"),
		addrsMap:  make(map[string]struct{}),
		coinTypes: make(map[string]struct{}),
		deposits:  make(chan DepositNote, 100),
	}
}

// RegisterCoinType marks a coinType as valid
func (s *DummyScanner) RegisterCoinType(coinType string) {
	s.Lock()
	defer s.Unlock()
	s.coinTypes[coinType] = struct{}{}
}

// AddScanAddress adds an address
func (s *DummyScanner) AddScanAddress(addr, coinType string) error {
	s.Lock()
	defer s.Unlock()

	if _, ok := s.coinTypes[coinType]; !ok {
		return fmt.Errorf("Invalid coin type \"%s\"", coinType)
	}

	if _, ok := s.addrsMap[addr]; ok {
		return NewDuplicateDepositAddressErr(addr)
	}

	s.addrsMap[addr] = struct{}{}
	s.addrs = append(s.addrs, addr)

	return nil
}

// GetScanAddresses returns all scan addresses
func (s *DummyScanner) GetScanAddresses() ([]string, error) {
	s.RLock()
	defer s.RUnlock()

	addrs := make([]string, len(s.addrs))
	copy(addrs, s.addrs)

	return addrs, nil
}

// GetDeposit returns a scanned deposit
func (s *DummyScanner) GetDeposit() <-chan DepositNote {
	return s.deposits
}

// HTTP Interface

// BindHandlers binds dummy scanner HTTP handlers
func (s *DummyScanner) BindHandlers(mux *http.ServeMux) {
	mux.Handle("/dummy/scanner/deposit", http.HandlerFunc(s.addDepositHandler))
}

func (s *DummyScanner) addDepositHandler(w http.ResponseWriter, r *http.Request) {
	coinType := r.FormValue("coin")
	if coinType == "" {
		coinType = CoinTypeBTC
	}

	addr := r.FormValue("addr")
	if addr == "" {
		httputil.ErrResponse(w, http.StatusBadRequest, "addr required")
		return
	}

	if _, err := cipher.BitcoinDecodeBase58Address(addr); err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "invalid addr")
		return
	}

	valueStr := r.FormValue("value")
	if valueStr == "" {
		httputil.ErrResponse(w, http.StatusBadRequest, "value required")
		return
	}

	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil || value < 0 {
		httputil.ErrResponse(w, http.StatusBadRequest, "invalid value")
		return
	}

	heightStr := r.FormValue("height")
	if heightStr == "" {
		httputil.ErrResponse(w, http.StatusBadRequest, "height required")
		return
	}

	height, err := strconv.ParseInt(heightStr, 10, 64)
	if err != nil || height < 0 {
		httputil.ErrResponse(w, http.StatusBadRequest, "invalid height")
		return
	}

	tx := r.FormValue("tx")
	if tx == "" {
		httputil.ErrResponse(w, http.StatusBadRequest, "tx required")
		return
	}

	var n uint32
	nStr := r.FormValue("n")
	if nStr != "" {
		n64, err := strconv.ParseInt(nStr, 10, 64)
		if err != nil || n64 < 0 {
			httputil.ErrResponse(w, http.StatusBadRequest, "invalid n")
			return
		}

		if n64 > math.MaxUint32 {
			httputil.ErrResponse(w, http.StatusBadRequest, "n too large")
			return
		}

		n = uint32(n64)
	}

	select {
	case s.deposits <- NewDepositNote(Deposit{
		CoinType: coinType,
		Address:  addr,
		Value:    value,
		Height:   height,
		Tx:       tx,
		N:        n,
	}):
	default:
		httputil.ErrResponse(w, http.StatusServiceUnavailable, "deposits channel is full")
		return
	}
}
