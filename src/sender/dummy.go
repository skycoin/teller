package sender

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/skycoin/skycoin/src/api/cli"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/skycoin/src/coin"
	"github.com/skycoin/skycoin/src/util/droplet"

	"github.com/skycoin/teller/src/util/httputil"
)

const seed = "survey tank about rely harbor client penalty antenna labor target jaguar bind"

func randSHA256() (cipher.SHA256, error) {
	b := make([]byte, 128)
	_, err := rand.Read(b)
	if err != nil {
		return cipher.SHA256{}, err
	}
	return cipher.SumSHA256(b), nil
}

// DummyTransaction wraps a *coin.Transaction with metadata for DummySender
type DummyTransaction struct {
	*coin.Transaction
	Confirmed bool
	Seq       int64
}

// DummySender implements the Exchanger interface in order to simulate
// SKY sendouts
type DummySender struct {
	broadcastTxns map[string]*DummyTransaction
	seq           int64
	secKey        cipher.SecKey
	log           logrus.FieldLogger
	sync.RWMutex
	coins uint64
	hours uint64
}

// NewDummySender creates a DummySender
func NewDummySender(log logrus.FieldLogger) *DummySender {
	_, sec := cipher.GenerateDeterministicKeyPair([]byte(seed))

	return &DummySender{
		broadcastTxns: make(map[string]*DummyTransaction),
		secKey:        sec,
		log:           log.WithField("prefix", "sender.dummy"),
		coins:         100000000,
		hours:         100,
	}
}

// CreateTransaction creates a fake skycoin transaction
func (s *DummySender) CreateTransaction(addr string, coins uint64) (*coin.Transaction, error) {
	if coins > s.coins {
		return nil, NewRPCError(errors.New("CreateTransaction not enough coins"))
	}

	c, err := droplet.ToString(coins)
	if err != nil {
		s.log.WithError(err).Error("droplet.ToString failed")
		return nil, err
	}

	s.log.WithFields(logrus.Fields{
		"addr":     addr,
		"droplets": coins,
		"coins":    c,
	}).Info("CreateTransaction")

	a, err := cipher.DecodeBase58Address(addr)
	if err != nil {
		s.log.WithError(err).Error("CreateTransaction called with invalid address")
		return nil, err
	}

	randomInput, err := randSHA256()
	if err != nil {
		s.log.WithError(err).Error("randSHA256 failed")
		return nil, err
	}

	txn := &coin.Transaction{}
	txn.PushInput(randomInput)
	txn.PushOutput(a, coins, 0)
	txn.SignInputs([]cipher.SecKey{s.secKey})
	return txn, nil
}

// BroadcastTransaction broadcasts a fake skycoin transaction
func (s *DummySender) BroadcastTransaction(txn *coin.Transaction) *BroadcastTxResponse {
	s.log.WithField("txid", txn.TxIDHex()).Info("BroadcastTransaction")

	s.Lock()
	defer s.Unlock()

	req := BroadcastTxRequest{
		Tx:   txn,
		RspC: make(chan *BroadcastTxResponse, 1),
	}

	if _, ok := s.broadcastTxns[txn.TxIDHex()]; ok {
		return &BroadcastTxResponse{
			Err: fmt.Errorf("Transaction %s was already broadcast", txn.TxIDHex()),
			Req: req,
		}
	}

	s.broadcastTxns[txn.TxIDHex()] = &DummyTransaction{
		Transaction: txn,
		Confirmed:   false,
		Seq:         s.seq,
	}

	s.seq++

	for _, output := range txn.Out {
		s.coins -= output.Coins
	}

	return &BroadcastTxResponse{
		Txid: txn.TxIDHex(),
		Req:  req,
	}
}

// IsTxConfirmed reports whether a fake skycoin transaction has been confirmed
func (s *DummySender) IsTxConfirmed(txid string) *ConfirmResponse {
	s.log.WithField("txid", txid).Info("IsTxConfirmed")

	s.RLock()
	defer s.RUnlock()

	txn := s.broadcastTxns[txid]

	return &ConfirmResponse{
		Confirmed: txn != nil && txn.Confirmed,
		Err:       nil,
		Req: ConfirmRequest{
			Txid: txid,
			RspC: make(chan *ConfirmResponse, 1),
		},
	}
}

// Balance returns the remaining balance
func (s *DummySender) Balance() (*cli.Balance, error) {

	coinStr, err := droplet.ToString(s.coins)

	if err != nil {
		return nil, err
	}

	return &cli.Balance{
		Coins: coinStr,
		Hours: fmt.Sprintf("%d", s.hours),
	}, nil
}

// HTTP interface

// BindHandlers binds admin API handlers to the mux
func (s *DummySender) BindHandlers(mux *http.ServeMux) {
	mux.Handle("/dummy/sender/broadcasts", http.HandlerFunc(s.getBroadcastedTransactionsHandler))
	mux.Handle("/dummy/sender/confirm", http.HandlerFunc(s.confirmBroadcastedTransactionHandler))
}

func (s *DummySender) getBroadcastedTransactions() []*DummyTransaction {
	s.RLock()
	defer s.RUnlock()

	txns := make([]*DummyTransaction, 0, len(s.broadcastTxns))
	for _, txn := range s.broadcastTxns {
		txns = append(txns, txn)
	}

	sort.Slice(txns, func(i, j int) bool {
		return txns[i].Seq < txns[j].Seq
	})

	return txns
}

type dummyTransactionResponseOutput struct {
	Address string `json:"address"`
	Coins   string `json:"coins"`
}

type dummyTransactionResponse struct {
	Txid      string                           `json:"txid"`
	Outputs   []dummyTransactionResponseOutput `json:"outputs"`
	Confirmed bool                             `json:"confirmed"`
}

func (s *DummySender) getBroadcastedTransactionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputil.ErrResponse(w, http.StatusMethodNotAllowed)
		return
	}

	txns := s.getBroadcastedTransactions()

	txnsRsp := make([]dummyTransactionResponse, 0, len(txns))
	for _, txn := range txns {
		outs := make([]dummyTransactionResponseOutput, 0, len(txn.Out))
		for _, o := range txn.Out {
			coins, err := droplet.ToString(o.Coins)
			if err != nil {
				s.log.WithError(err).Error("getBroadcastedTransactionsHandler: droplet.ToString failed")
				httputil.ErrResponse(w, http.StatusInternalServerError)
				return
			}

			outs = append(outs, dummyTransactionResponseOutput{
				Address: o.Address.String(),
				Coins:   coins,
			})
		}

		txnsRsp = append(txnsRsp, dummyTransactionResponse{
			Txid:      txn.TxIDHex(),
			Confirmed: txn.Confirmed,
			Outputs:   outs,
		})
	}

	if err := httputil.JSONResponse(w, txnsRsp); err != nil {
		s.log.WithError(err).Error(err)
	}
}

func (s *DummySender) confirmBroadcastedTransactionHandler(w http.ResponseWriter, r *http.Request) {
	txid := r.FormValue("txid")
	seqStr := r.FormValue("seq")
	if seqStr == "" && txid == "" {
		httputil.ErrResponse(w, http.StatusBadRequest, "txid or seq required")
		return
	}

	var seq int64
	if seqStr != "" {
		var err error
		seq, err = strconv.ParseInt(seqStr, 10, 64)
		if err != nil {
			httputil.ErrResponse(w, http.StatusBadRequest, "invalid seq")
			return
		}
	}

	s.Lock()
	defer s.Unlock()

	var txn *DummyTransaction
	if txid != "" {
		txn = s.broadcastTxns[txid]
	} else if seqStr != "" {
		for _, t := range s.broadcastTxns {
			if t.Seq == seq {
				txn = t
				break
			}
		}
	} else {
		s.log.Error("txid and seq both empty, should have caught this condition earlier")
		httputil.ErrResponse(w, http.StatusInternalServerError)
		return
	}

	if txn == nil {
		httputil.ErrResponse(w, http.StatusNotFound, "txn not found")
		return
	}

	txn.Confirmed = true
}
