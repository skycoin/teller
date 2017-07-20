// Package scanner scans bitcoin blockchain and check all transactions
// to see if there're addresses in vout that can match our deposit addresses
// if find, then generate an event and push to deposite even channel
package scanner

import (
	"fmt"
	"io/ioutil"
	"log"

	"time"

	"github.com/boltdb/bolt"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcrpcclient"
	"github.com/skycoin/teller/src/logger"
)

// Scanner blockchain scanner to check if there're deposit coins
type Scanner struct {
	logger.Logger
	cfg       Config
	btcrpcclt *btcrpcclient.Client
	store     *store
	depositC  chan DepositValue // deposit value channel
	quit      chan struct{}
}

// Config scanner config info
type Config struct {
	Log logger.Logger
	DB  *bolt.DB

	RPCServer          string // btcd rpc server address
	RPCUser            string // btcd rpc user
	RPCPass            string // btcd rpc pass
	CertPath           string // cert file path
	ScanPeriod         int64  // scan period in seconds
	DepositChanBufsize int32  // deposit channel buffer size
}

// DepositValue struct
type DepositValue struct {
	Address string  // deposit address
	Value   float64 // deposit coins
	Tx      string  // the transaction id
	N       uint32  // the index of vout in the tx
}

// New creates scanner instance
func New(cfg Config) (*Scanner, error) {
	s, err := newStore(cfg.DB)
	if err != nil {
		return nil, err
	}
	return &Scanner{
		Logger:   cfg.Log,
		cfg:      cfg,
		store:    s,
		depositC: make(chan DepositValue, cfg.DepositChanBufsize),
		quit:     make(chan struct{}),
	}, nil
}

// Run starts the scanner
func (scan *Scanner) Run() error {
	scan.Println("Bitcoin blockchain scanner start...")

	// connect to btcd
	var err error
	scan.btcrpcclt, err = connectBTCD(scan.cfg.RPCServer,
		scan.cfg.RPCUser,
		scan.cfg.RPCPass,
		scan.cfg.CertPath)
	if err != nil {
		return fmt.Errorf("Connect to btcd failed: %v", err)
	}

	scan.Println("Connect to btcd success")

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

		if err := scan.scanBlock(block); err != nil {
			return fmt.Errorf("Scan block %s failed: %v", block.Hash, err)
		}
	}

	errC := make(chan error)
	go func() {
		for {
			nxtBlock, err := scan.getNextBlock(hash)
			if err != nil {
				errC <- err
				return
			}

			if nxtBlock == nil {
				scan.Println("No new block to scan...")
				time.Sleep(time.Duration(scan.cfg.ScanPeriod) * time.Second)
				continue
			}

			block = nxtBlock
			hash = block.Hash
			if err := scan.scanBlock(block); err != nil {
				errC <- fmt.Errorf("Scan block %s failed: %v", block.Hash, err)
				return
			}
		}
	}()

	select {
	case err := <-errC:
		return err
	case <-scan.quit:
		return nil
	}
}

func (scan *Scanner) scanBlock(block *btcjson.GetBlockVerboseResult) error {
	addrs, err := scan.getDepositAddresses()
	if err != nil {
		return err
	}

	dvs := scanBlock(block, addrs)
	for _, dv := range dvs {
		select {
		case scan.depositC <- dv:
		case <-scan.quit:
			// scanner was closed
			return nil
		}
	}
	return nil
}

// scanBlock scan the given block and returns the next block hash or error
func scanBlock(block *btcjson.GetBlockVerboseResult, depositAddrs []string) []DepositValue {
	addrMap := map[string]struct{}{}
	for _, a := range depositAddrs {
		addrMap[a] = struct{}{}
	}

	var dv []DepositValue
	for _, tx := range block.RawTx {
		for _, v := range tx.Vout {
			for _, a := range v.ScriptPubKey.Addresses {
				if _, ok := addrMap[a]; ok {
					dv = append(dv, DepositValue{
						Address: a,
						Value:   v.Value,
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
func (scan *Scanner) AddDepositAddress(addr string) error {
	return scan.store.addDepositAddress(addr)
}

// GetBestBlock returns the hash and height of the block in the longest (best)
// chain.
func (scan *Scanner) getBestBlock() (*btcjson.GetBlockVerboseResult, error) {
	hash, _, err := scan.btcrpcclt.GetBestBlock()
	if err != nil {
		return nil, err
	}

	return scan.btcrpcclt.GetBlockVerbose(hash)
}

// getBlock returns block of given hash
func (scan *Scanner) getBlock(hash *chainhash.Hash) (*btcjson.GetBlockVerboseResult, error) {
	return scan.btcrpcclt.GetBlockVerbose(hash)
}

// getNextBlock returns the next block of given hash, return nil if next block does not exist
func (scan *Scanner) getNextBlock(hash string) (*btcjson.GetBlockVerboseResult, error) {
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

	nextHash, err := chainhash.NewHashFromStr(b.NextHash)
	if err != nil {
		return nil, err
	}

	return scan.getBlock(nextHash)
}

// getNextBlockByHashStr returns next block of given block hash string
// func (scan *Scanner) getNextBlockByHashStr(hash string) (*btcjson.GetBlockVerboseResult, error) {
// 	h, err := chainhash.NewHashFromStr(hash)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return scan.getNextBlock(h)
// }

// setLastScanBlock sets the last scan block hash and height
func (scan *Scanner) setLastScanBlock(hash *chainhash.Hash, height int32) error {
	return scan.store.setLastScanBlock(lastScanBlock{
		Hash:   hash.String(),
		Height: height,
	})
}

// getLastScanBlock returns the last scanned block hash and height
func (scan *Scanner) getLastScanBlock() (string, int32, error) {
	return scan.store.getLastScanBlock()
}

// getDepositAddresses returns the deposit addresses that need to scan
func (scan *Scanner) getDepositAddresses() ([]string, error) {
	return scan.store.getDepositAddresses()
}

// Shutdown shutdown the scanner
func (scan *Scanner) Shutdown() {
	scan.btcrpcclt.Shutdown()
	close(scan.quit)
}

func connectBTCD(server, user, pass, certPath string) (*btcrpcclient.Client, error) {
	// connect to the btcd
	certs, err := ioutil.ReadFile(certPath)
	if err != nil {
		log.Fatal(err)
	}
	connCfg := &btcrpcclient.ConnConfig{
		Host:         "localhost:8334",
		Endpoint:     "ws",
		User:         user,
		Pass:         pass,
		Certificates: certs,
	}
	return btcrpcclient.New(connCfg, nil)
}
