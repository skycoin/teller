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

	"time"

	"github.com/boltdb/bolt"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcrpcclient"
	"github.com/skycoin/teller/src/logger"
)

var (
	errBlockNotFound = errors.New("Block not found")
)

// Btcrpcclient rpcclient interface
type Btcrpcclient interface {
	GetBestBlock() (*chainhash.Hash, int32, error)
	GetBlockVerboseTx(blockHash *chainhash.Hash) (*btcjson.GetBlockVerboseResult, error)
	Shutdown()
}

// ScanService blockchain scanner to check if there're deposit coins
type ScanService struct {
	logger.Logger
	cfg       Config
	btcrpcclt Btcrpcclient
	store     *store
	depositC  chan DepositValue // deposit value channel
	quit      chan struct{}
}

// Config scanner config info
type Config struct {
	ScanPeriod        time.Duration // scan period in seconds
	DepositBuffersize uint32        // deposit channel buffer size
}

// DepositValue struct
type DepositValue struct {
	Address string  // deposit address
	Value   float64 // deposit coins
	Height  int64   // the block height
	Tx      string  // the transaction id
	N       uint32  // the index of vout in the tx
}

// NewService creates scanner instance
func NewService(cfg Config, db *bolt.DB, log logger.Logger, btcrpcclient Btcrpcclient) (*ScanService, error) {
	s, err := newStore(db)
	if err != nil {
		return nil, err
	}
	return &ScanService{
		btcrpcclt: btcrpcclient,
		Logger:    log,
		cfg:       cfg,
		store:     s,
		depositC:  make(chan DepositValue, cfg.DepositBuffersize),
		quit:      make(chan struct{}),
	}, nil
}

// Run starts the scanner
func (scan *ScanService) Run() error {
	scan.Println("Start bitcoin blockchain scan service...")
	defer scan.Println("Bitcoin blockchain scan service closed")

	// get last scan block
	hash, height, err := scan.getLastScanBlock()
	if err != nil {
		return fmt.Errorf("get last scan block failed: %v", err)
	}

	var block *btcjson.GetBlockVerboseResult

	if height == 0 {
		// the first time the bot start
		// get the best block
		block, err = scan.getBestBlock()
		if err != nil {
			return err
		}

		hash = block.Hash

		if err := scan.scanBlock(block); err != nil {
			return fmt.Errorf("Scan block %s failed: %v", block.Hash, err)
		}
	}

	for {
		nxtBlock, err := scan.getNextBlock(hash)
		if err != nil {
			select {
			case <-scan.quit:
				return nil
			default:
				return err
			}
		}

		if nxtBlock == nil {
			scan.Println("No new block to scan...")
			select {
			case <-scan.quit:
				return nil
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
				return nil
			default:
				return fmt.Errorf("Scan block %s failed: %v", block.Hash, err)
			}
		}
	}
}

func (scan *ScanService) scanBlock(block *btcjson.GetBlockVerboseResult) error {
	addrs, err := scan.getDepositAddresses()
	if err != nil {
		return err
	}

	dvs := scanBlock(block, addrs)
	for _, dv := range dvs {
		select {
		case scan.depositC <- dv:
			// scan.Println("push dv:", dv)
		case <-scan.quit:
			// scanner was closed
			return nil
		}
	}

	hash, err := chainhash.NewHashFromStr(block.Hash)
	if err != nil {
		return err
	}

	return scan.setLastScanBlock(hash, block.Height)
}

// scanBlock scan the given block and returns the next block hash or error
func scanBlock(block *btcjson.GetBlockVerboseResult, depositAddrs []string) []DepositValue {
	addrMap := map[string]struct{}{}
	for _, a := range depositAddrs {
		addrMap[a] = struct{}{}
	}

	var dv []DepositValue
	for _, tx := range block.RawTx {
		// fmt.Println("tx:", tx.Txid)
		for _, v := range tx.Vout {
			for _, a := range v.ScriptPubKey.Addresses {
				// fmt.Println("\taddr:", a)
				if _, ok := addrMap[a]; ok {
					dv = append(dv, DepositValue{
						Address: a,
						Value:   v.Value,
						Height:  block.Height,
						Tx:      tx.Txid,
						N:       v.N,
					})
				}
			}
		}
	}

	return dv
}

// AddDepositAddress adds new deposit address
func (scan *ScanService) AddDepositAddress(addr string) error {
	return scan.store.addDepositAddress(addr)
}

// GetBestBlock returns the hash and height of the block in the longest (best)
// chain.
func (scan *ScanService) getBestBlock() (*btcjson.GetBlockVerboseResult, error) {
	hash, _, err := scan.btcrpcclt.GetBestBlock()
	if err != nil {
		return nil, err
	}

	return scan.getBlock(hash)
}

// getBlock returns block of given hash
func (scan *ScanService) getBlock(hash *chainhash.Hash) (*btcjson.GetBlockVerboseResult, error) {
	return scan.btcrpcclt.GetBlockVerboseTx(hash)
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
	return scan.store.setLastScanBlock(lastScanBlock{
		Hash:   hash.String(),
		Height: height,
	})
}

// getLastScanBlock returns the last scanned block hash and height
func (scan *ScanService) getLastScanBlock() (string, int64, error) {
	return scan.store.getLastScanBlock()
}

// getDepositAddresses returns the deposit addresses that need to scan
func (scan *ScanService) getDepositAddresses() ([]string, error) {
	return scan.store.getDepositAddresses()
}

// Shutdown shutdown the scanner
func (scan *ScanService) Shutdown() {
	close(scan.quit)
	scan.btcrpcclt.Shutdown()
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
