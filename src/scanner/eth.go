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
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/sirupsen/logrus"
)

// ETHScanner blockchain scanner to check if there're deposit coins
type ETHScanner struct {
	log       logrus.FieldLogger
	ethClient EthRPCClient
	Base      *BaseScanner
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
	return s.Base.Run(s.ethClient.GetBlockCount, s.getBlockAtHeight, s.getBlockHashAndHeight, s.waitForNextBlock, s.scanBlock)
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
func (s *ETHScanner) scanBlock(blk interface{}) (int, error) {
	block := blk.(*types.Block)
	log := s.log.WithField("hash", block.Hash().String())
	log = log.WithField("height", block.NumberU64())

	log.Debug("Scanning block")

	dvs, err := s.Base.Store.ScanBlock(block, CoinTypeETH)
	if err != nil {
		log.WithError(err).Error("store.ScanBlock failed")
		return 0, err
	}

	log = log.WithField("scannedDeposits", len(dvs))
	log.Infof("Counted %d deposits from block", len(dvs))

	n := 0
	for _, dv := range dvs {
		select {
		case s.Base.ScannedDeposits <- dv:
			n++
		case <-s.Base.Quit:
			return n, errQuit
		}
	}

	return n, nil
}

// getBlockAtHeight returns that block at a specific height
func (s *ETHScanner) getBlockAtHeight(seq int64) (interface{}, error) {
	return s.ethClient.GetBlockVerboseTx(uint64(seq))
}

// getNextBlock returns the next block of given hash, return nil if next block does not exist
func (s *ETHScanner) getNextBlock(seq uint64) (*types.Block, error) {
	return s.ethClient.GetBlockVerboseTx(seq + 1)
}

// waitForNextBlock scans for the next block until it is available
func (s *ETHScanner) waitForNextBlock(blk interface{}) (interface{}, error) {
	block := blk.(*types.Block)
	log := s.log.WithField("blockHash", block.Hash().String())
	log = log.WithField("blockHeight", block.NumberU64())
	log.Debug("Waiting for the next block")

	for {
		nextBlock, err := s.getNextBlock(block.NumberU64())
		if err != nil {
			log.WithError(err).Error("getNextBlock failed")
		}
		if nextBlock == nil {
			log.Debug("No new block yet")
		}
		if err != nil || nextBlock == nil {
			select {
			case <-s.Base.Quit:
				return nil, errQuit
			case <-time.After(s.Base.GetScanPeriod()):
				continue
			}
		}

		log.WithFields(logrus.Fields{
			"hash":   nextBlock.Hash().String(),
			"height": nextBlock.NumberU64(),
		}).Debug("Found nextBlock")

		return nextBlock, nil
	}
}

// AddScanAddress adds new scan address
func (s *ETHScanner) AddScanAddress(addr string) error {
	return s.Base.Store.AddScanAddress(addr, CoinTypeETH)
}

// GetScanAddresses returns the deposit addresses that need to scan
func (s *ETHScanner) GetScanAddresses() ([]string, error) {
	return s.Base.Store.GetScanAddresses(CoinTypeETH)
}

// GetDeposit returns deposit value channel.
func (s *ETHScanner) GetDeposit() <-chan DepositNote {
	return s.Base.DepositC
}

//EthClient is self-defined struct for implement EthRPCClient interface
//because origin rpc.Client has't required interface
type EthClient struct {
	c *rpc.Client
}

//NewEthClient create ethereum rpc client
func NewEthClient(server, port string) (*EthClient, error) {
	ethrpc, err := rpc.Dial("http://" + server + ":" + port)
	if err != nil {
		return nil, err
	}
	ec := EthClient{c: ethrpc}
	return &ec, nil
}

//GetBlockCount returns ethereum block count
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
	return int64(blockNum), nil
}

func (s *ETHScanner) getBlockHashAndHeight(block interface{}) (string, int64) {
	b := block.(*types.Block)
	return b.Hash().String(), int64(b.NumberU64())
}

//Shutdown close rpc connection
func (ec *EthClient) Shutdown() {
	ec.c.Close()
}

//GetBlockVerboseTx returns ethereum block data
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
