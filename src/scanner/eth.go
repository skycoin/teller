// Package scanner scans ethcoin blockchain and check all transactions
// to see if there are addresses in vout that can match our deposit addresses.
// If found, then generate an event and push to deposit event channel
//
// current scanner doesn't support reconnect after btcd shutdown, if
// any error occur when call btcd apis, the scan service will be closed.
package scanner

import (
	"context"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/sirupsen/logrus"

	"github.com/skycoin/teller/src/util/mathutil"
)

// ETHScanner blockchain scanner to check if there're deposit coins
type ETHScanner struct {
	log       logrus.FieldLogger
	ethClient EthRPCClient
	Base      CommonScanner
}

// NewETHScanner creates scanner instance
func NewETHScanner(log logrus.FieldLogger, store Storer, eth EthRPCClient, cfg Config) (*ETHScanner, error) {
	bs := NewBaseScanner(store, log.WithField("prefix", "scanner.eth"), cfg)

	return &ETHScanner{
		ethClient: eth,
		log:       log.WithField("prefix", "scanner.eth"),
		Base:      bs,
	}, nil
}

// Run starts the scanner
func (s *ETHScanner) Run() error {
	return s.Base.Run(s.ethClient.GetBlockCount, s.getBlockAtHeight, s.waitForNextBlock, s.scanBlock)
}

// Shutdown shutdown the scanner
func (s *ETHScanner) Shutdown() {
	s.log.Info("Closing ETH scanner")
	s.ethClient.Shutdown()
	s.Base.Shutdown()
	s.log.Info("Waiting for ETH scanner to stop")
	s.log.Info("ETH scanner stopped")
}

// scanBlock scans for a new ETH block every ScanPeriod.
// When a new block is found, it compares the block against our scanning
// deposit addresses. If a matching deposit is found, it saves it to the DB.
func (s *ETHScanner) scanBlock(block *CommonBlock) (int, error) {
	log := s.log.WithField("hash", block.Hash)
	log = log.WithField("height", block.Height)

	log.Debug("Scanning block")

	dvs, err := s.Base.GetStorer().ScanBlock(block, CoinTypeETH)
	if err != nil {
		log.WithError(err).Error("store.ScanBlock failed")
		return 0, err
	}

	log = log.WithField("scannedDeposits", len(dvs))
	log.Infof("Counted %d deposits from block", len(dvs))

	n := 0
	for _, dv := range dvs {
		select {
		case s.Base.GetScannedDepositChan() <- dv:
			n++
		case <-s.Base.GetQuitChan():
			return n, errQuit
		}
	}

	return n, nil
}

// getBlockAtHeight returns that block at a specific height
func (s *ETHScanner) getBlockAtHeight(seq int64) (*CommonBlock, error) {
	b, err := s.ethClient.GetBlockVerboseTx(uint64(seq))
	if err != nil {
		return nil, err
	}
	return ethBlock2CommonBlock(b)
}

// getNextBlock returns the next block of given hash, return nil if next block does not exist
func (s *ETHScanner) getNextBlock(seq uint64) (*CommonBlock, error) {
	b, err := s.ethClient.GetBlockVerboseTx(seq + 1)
	if err != nil {
		return nil, err
	}
	return ethBlock2CommonBlock(b)
}

// waitForNextBlock scans for the next block until it is available
func (s *ETHScanner) waitForNextBlock(block *CommonBlock) (*CommonBlock, error) {
	log := s.log.WithField("blockHash", block.Hash)
	log = log.WithField("blockHeight", block.Height)
	log.Debug("Waiting for the next block")

	for {
		nextBlock, err := s.getNextBlock(uint64(block.Height))
		if err != nil {
			log.WithError(err).Error("getNextBlock failed")
		}
		if nextBlock == nil {
			log.Debug("No new block yet")
		}
		if err != nil || nextBlock == nil {
			select {
			case <-s.Base.GetQuitChan():
				return nil, errQuit
			case <-time.After(s.Base.GetScanPeriod()):
				continue
			}
		}

		log.WithFields(logrus.Fields{
			"hash":   nextBlock.Hash,
			"height": nextBlock.Height,
		}).Debug("Found nextBlock")

		return nextBlock, nil
	}
}

// AddScanAddress adds new scan address
func (s *ETHScanner) AddScanAddress(addr, coinType string) error {
	return s.Base.GetStorer().AddScanAddress(addr, coinType)
}

// GetScanAddresses returns the deposit addresses that need to scan
func (s *ETHScanner) GetScanAddresses() ([]string, error) {
	return s.Base.GetStorer().GetScanAddresses(CoinTypeETH)
}

// GetDeposit returns deposit value channel.
func (s *ETHScanner) GetDeposit() <-chan DepositNote {
	return s.Base.GetDeposit()
}

// ethBlock2CommonBlock convert ethereum block to common block
func ethBlock2CommonBlock(block *types.Block) (*CommonBlock, error) {
	cb := CommonBlock{}
	cb.Hash = block.Hash().String()
	cb.Height = int64(block.NumberU64())
	cb.RawTx = make([]CommonTx, 0, len(block.Transactions()))
	for i, tx := range block.Transactions() {
		to := tx.To()
		if to == nil {
			//this is a contract transcation
			continue
		}
		cbTx := CommonTx{}
		cbTx.Txid = tx.Hash().String()
		cbTx.Vout = make([]CommonVout, 0, 1)
		//1 eth = 1e18 wei ,tx.Value() is very big that may overflow(int64), so store it as Gwei(1Gwei=1e9wei) and recover it when used
		amt := mathutil.Wei2Gwei(tx.Value())

		//ethcoin address must be lowercase
		realaddr := strings.ToLower(to.String())
		cv := CommonVout{}
		cv.N = uint32(i)
		cv.Value = amt
		cv.Addresses = []string{realaddr}
		cbTx.Vout = append(cbTx.Vout, cv)
		cb.RawTx = append(cb.RawTx, cbTx)
	}
	return &cb, nil
}

// EthClient is self-defined struct for implement EthRPCClient interface
// because origin rpc.Client has't required interface
type EthClient struct {
	c *rpc.Client
}

// NewEthClient create ethereum rpc client
func NewEthClient(server, port string) (*EthClient, error) {
	ethrpc, err := rpc.Dial("http://" + server + ":" + port)
	if err != nil {
		return nil, err
	}
	ec := EthClient{c: ethrpc}
	return &ec, nil
}

// GetBlockCount returns ethereum block count
func (ec *EthClient) GetBlockCount() (int64, error) {
	var bn string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := ec.c.CallContext(ctx, &bn, "eth_blockNumber"); err != nil {
		return 0, err
	}
	bnRealStr := bn[2:]
	blockNum, err := strconv.ParseInt(bnRealStr, 16, 32)
	if err != nil {
		return 0, err
	}
	return blockNum, nil
}

// Shutdown close rpc connection
func (ec *EthClient) Shutdown() {
	ec.c.Close()
}

// GetBlockVerboseTx returns ethereum block data
func (ec *EthClient) GetBlockVerboseTx(seq uint64) (*types.Block, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	block, err := ethclient.NewClient(ec.c).BlockByNumber(ctx, big.NewInt(int64(seq)))
	if err != nil && err.Error() != "not found" {
		return nil, err
	}

	return block, nil
}

//GetTransaction returns transaction by txhash
func (ec *EthClient) GetTransaction(txhash common.Hash) (*types.Transaction, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	//txhash := common.StringToHash(txid)
	tx, _, err := ethclient.NewClient(ec.c).TransactionByHash(ctx, txhash)
	if err != nil {
		return nil, err
	}
	return tx, nil
}
