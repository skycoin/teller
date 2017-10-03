// Package scanner scans bitcoin blockchain and check all transactions
// to see if there're addresses in vout that can match our deposit addresses
// if find, then generate an event and push to deposite even channel
//
// current scanner doesn't support reconnect after btcd shutdown, if
// any error occur when call btcd apis, the scan service will be closed.
package scanner

import (
	"errors"
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcrpcclient"
	"github.com/btcsuite/btcutil"

	"github.com/skycoin/teller/src/logger"
)

var (
	errBlockNotFound = errors.New("block not found")
)

const (
	checkHeadDepositValuePeriod = time.Second * 5
)

// BtcRPCClient rpcclient interface
type BtcRPCClient interface {
	GetBestBlock() (*chainhash.Hash, int32, error)
	GetBlockVerboseTx(blockHash *chainhash.Hash) (*btcjson.GetBlockVerboseResult, error)
	Shutdown()
}

type DepositNote struct {
	DepositValue
	AckC chan struct{}
}

func makeDepositNote(dv DepositValue) DepositNote {
	return DepositNote{
		DepositValue: dv,
		AckC:         make(chan struct{}, 1),
	}
}

// ScanService blockchain scanner to check if there're deposit coins
type ScanService struct {
	logger.Logger
	cfg       Config
	btcClient BtcRPCClient
	store     *store
	depositC  chan DepositNote // deposit value channel
	quit      chan struct{}
}

// Config scanner config info
type Config struct {
	ScanPeriod        time.Duration // scan period in seconds
	DepositBuffersize uint32        // deposit channel buffer size
}

// DepositValue struct
type DepositValue struct {
	Address string // deposit address
	Value   int64  // deposit BTC amount, in satoshis
	Height  int64  // the block height
	Tx      string // the transaction id
	N       uint32 // the index of vout in the tx
	IsUsed  bool   // whether this dv is used
}

// TxN returns $tx:$n formatted ID string
func (d DepositValue) TxN() string {
	return fmt.Sprintf("%s:%d", d.Tx, d.N)
}

// NewService creates scanner instance
func NewService(cfg Config, db *bolt.DB, log logger.Logger, btc BtcRPCClient) (*ScanService, error) {
	s, err := newStore(db)
	if err != nil {
		return nil, err
	}
	return &ScanService{
		btcClient: btc,
		Logger:    log,
		cfg:       cfg,
		store:     s,
		depositC:  make(chan DepositNote),
		quit:      make(chan struct{}),
	}, nil
}

// Run starts the scanner
func (scan *ScanService) Run() error {
	scan.Println("Start bitcoin blockchain scan service...")
	defer scan.Println("Bitcoin blockchain scan service closed")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			headDv, err := scan.store.getHeadDepositValue()
			if err != nil {
				switch err.(type) {
				case DepositValuesEmptyErr:
					select {
					case <-time.After(checkHeadDepositValuePeriod):
						continue
					case <-scan.quit:
						return
					}
				default:
					scan.Println("getHeadDepositValue failed:", err)
					return
				}
			}

			dn := makeDepositNote(headDv)
			select {
			case <-scan.quit:
				return
			case scan.depositC <- dn:
				select {
				case <-dn.AckC:
					// pop the head deposit value in store
					if ddv, err := scan.store.popDepositValue(); err != nil {
						scan.Println("pop deposit value failed:", err)
					} else {
						scan.Debugf("deposit value: %+v is processed\n", ddv)
					}
				case <-scan.quit:
					return
				}
			}
		}
	}()

	// get last scan block
	lsb, err := scan.getLastScanBlock()
	if err != nil {
		return fmt.Errorf("get last scan block failed: %v", err)
	}

	height := lsb.Height
	hash := lsb.Hash

	var block *btcjson.GetBlockVerboseResult

	if height == 0 {
		// the first time the bot start
		// get the best block
		block, err = scan.getBestBlock()
		if err != nil {
			return err
		}

		if err := scan.scanBlock(block); err != nil {
			return fmt.Errorf("Scan block %s failed: %v", block.Hash, err)
		}

		hash = block.Hash
	}

	wg.Add(1)
	errC := make(chan error, 1)
	go func() {
		defer wg.Done()

		for {
			nxtBlock, err := scan.getNextBlock(hash)
			if err != nil {
				select {
				case <-scan.quit:
					return
				default:
					errC <- err
					return
				}
			}

			if nxtBlock == nil {
				scan.Debugln("No new block to scan...")
				select {
				case <-scan.quit:
					return
				case <-time.After(time.Duration(scan.cfg.ScanPeriod) * time.Second):
					continue
				}
			}

			block = nxtBlock
			hash = block.Hash
			height = block.Height

			scan.Debugf("scan height: %v hash:%s\n", height, hash)
			if err := scan.scanBlock(block); err != nil {
				select {
				case <-scan.quit:
					return
				default:
					errC <- fmt.Errorf("Scan block %s failed: %v", block.Hash, err)
					return
				}
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case err := <-errC:
		return err
	}
}

func (scan *ScanService) scanBlock(block *btcjson.GetBlockVerboseResult) error {
	return scan.store.db.View(func(tx *bolt.Tx) error {
		addrs, err := scan.store.getScanAddressesTx(tx)
		if err != nil {
			return err
		}

		dvs, err := scanBlock(block, addrs)
		if err != nil {
			return err
		}

		for _, dv := range dvs {
			if err := scan.store.pushDepositValueTx(tx, dv); err != nil {
				switch err.(type) {
				case DepositValueExistsErr:
					continue
				default:
					scan.Printf("Persist deposit value %+v failed: %v\n", dv, err)
				}
			}
		}

		hash, err := chainhash.NewHashFromStr(block.Hash)
		if err != nil {
			return err
		}

		return scan.store.setLastScanBlockTx(tx, LastScanBlock{
			Hash:   hash.String(),
			Height: block.Height,
		})
	})
}

// scanBlock scan the given block and returns the next block hash or error
func scanBlock(block *btcjson.GetBlockVerboseResult, depositAddrs []string) ([]DepositValue, error) {
	addrMap := map[string]struct{}{}
	for _, a := range depositAddrs {
		addrMap[a] = struct{}{}
	}

	var dv []DepositValue
	for _, tx := range block.RawTx {
		for _, v := range tx.Vout {
			amt, err := btcutil.NewAmount(v.Value)
			if err != nil {
				return nil, err
			}

			for _, a := range v.ScriptPubKey.Addresses {
				if _, ok := addrMap[a]; ok {
					dv = append(dv, DepositValue{
						Address: a,
						Value:   int64(amt),
						Height:  block.Height,
						Tx:      tx.Txid,
						N:       v.N,
					})
				}
			}
		}
	}

	return dv, nil
}

// AddScanAddress adds new scan address
func (scan *ScanService) AddScanAddress(addr string) error {
	return scan.store.addScanAddress(addr)
}

// GetBestBlock returns the hash and height of the block in the longest (best)
// chain.
func (scan *ScanService) getBestBlock() (*btcjson.GetBlockVerboseResult, error) {
	hash, _, err := scan.btcClient.GetBestBlock()
	if err != nil {
		return nil, err
	}

	return scan.getBlock(hash)
}

// getBlock returns block of given hash
func (scan *ScanService) getBlock(hash *chainhash.Hash) (*btcjson.GetBlockVerboseResult, error) {
	return scan.btcClient.GetBlockVerboseTx(hash)
}

// getNextBlock returns the next block of given hash, return nil if next block does not exist
func (scan *ScanService) getNextBlock(hash string) (*btcjson.GetBlockVerboseResult, error) {
	h, err := chainhash.NewHashFromStr(hash)
	if err != nil {
		return nil, err
	}

	b, err := scan.getBlock(h)
	if err != nil {
		return nil, err
	}

	if b.NextHash == "" {
		return nil, nil
	}

	nxtHash, err := chainhash.NewHashFromStr(b.NextHash)
	if err != nil {
		return nil, err
	}

	return scan.getBlock(nxtHash)
}

// setLastScanBlock sets the last scan block hash and height
func (scan *ScanService) setLastScanBlock(hash *chainhash.Hash, height int64) error {
	return scan.store.setLastScanBlock(LastScanBlock{
		Hash:   hash.String(),
		Height: height,
	})
}

// getLastScanBlock returns the last scanned block hash and height
func (scan *ScanService) getLastScanBlock() (LastScanBlock, error) {
	return scan.store.getLastScanBlock()
}

// getScanAddresses returns the deposit addresses that need to scan
func (scan *ScanService) getScanAddresses() ([]string, error) {
	return scan.store.getScanAddresses()
}

// Shutdown shutdown the scanner
func (scan *ScanService) Shutdown() {
	close(scan.quit)
	fmt.Println("Close scan service")
	scan.btcClient.Shutdown()
}

// ConnectBTCD connects to the btcd rpcserver
func ConnectBTCD(server, user, pass, certPath string) (*btcrpcclient.Client, error) {
	// connect to the btcd
	certs, err := ioutil.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	connCfg := &btcrpcclient.ConnConfig{
		Host:         server,
		Endpoint:     "ws",
		User:         user,
		Pass:         pass,
		Certificates: certs,
	}
	return btcrpcclient.New(connCfg, nil)
}
