// Package scanner scans waves blockchain and check all transactions
// to see if there are addresses in vout that can match our deposit addresses.
// If found, then generate an event and push to deposit event channel
//
package scanner

import (
	"time"

	"github.com/sirupsen/logrus"

	"github.com/modeneis/waves-go-client/client"
	"github.com/modeneis/waves-go-client/model"
)

// WAVESScanner blockchain scanner to check if there're deposit coins
type WAVESScanner struct {
	log            logrus.FieldLogger
	Base           CommonScanner
	wavesRPCClient WavesRPCClient
}

// NewWavescoinScanner creates scanner instance
func NewWavescoinScanner(log logrus.FieldLogger, store Storer, client WavesRPCClient, cfg Config) (*WAVESScanner, error) {
	bs := NewBaseScanner(store, log.WithField("prefix", "scanner.waves"), CoinTypeWAVES, cfg)

	return &WAVESScanner{
		wavesRPCClient: client,
		log:            log.WithField("prefix", "scanner.waves"),
		Base:           bs,
	}, nil
}

// Run starts the scanner
func (s *WAVESScanner) Run() error {
	return s.Base.Run(s.GetBlockCount, s.getBlockAtHeight, s.waitForNextBlock, s.scanBlock)
}

// Shutdown shutdown the scanner
func (s *WAVESScanner) Shutdown() {
	s.log.Info("Closing WAVES scanner")
	s.wavesRPCClient.Shutdown()
	s.Base.Shutdown()
	s.log.Info("Waiting for WAVES scanner to stop")
	s.log.Info("WAVES scanner stopped")
}

// scanBlock scans for a new WAVES block every ScanPeriod.
// When a new block is found, it compares the block against our scanning
// deposit addresses. If a matching deposit is found, it saves it to the DB.
func (s *WAVESScanner) scanBlock(block *CommonBlock) (int, error) {
	log := s.log.WithField("hash", block.Hash)
	log = log.WithField("height", block.Height)

	log.Debug("Scanning block")

	dvs, err := s.Base.GetStorer().ScanBlock(block, CoinTypeWAVES)
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

// wavesBlock2CommonBlock convert wavescoin block to common block
func wavesBlock2CommonBlock(block *model.Blocks) (*CommonBlock, error) {
	if block == nil {
		return nil, ErrEmptyBlock
	}
	cb := CommonBlock{}

	cb.Hash = block.Signature
	cb.Height = int64(block.Height)
	//cb.RawTx = make([]CommonTx, 0, len(block.Transactions))
	for _, tx := range block.Transactions {
		if tx.Recipient == "" {
			continue
		}
		cbTx := CommonTx{}
		cbTx.Txid = tx.ID
		//cbTx.Vout = make([]CommonVout, 0, 1)

		cv := CommonVout{}
		cv.N = uint32(0)
		cv.Value = tx.Amount
		cv.Addresses = []string{tx.Recipient}

		cbTx.Vout = append(cbTx.Vout, cv)
		cb.RawTx = append(cb.RawTx, cbTx)
	}

	return &cb, nil
}

// GetBlockCount returns the hash and height of the block in the longest (best) chain.
func (s *WAVESScanner) GetBlockCount() (int64, error) {
	rb, err := s.wavesRPCClient.GetLastBlocks()
	if err != nil {
		return 0, err
	}

	return int64(rb.Height), nil
}

// getBlock returns block of given hash
func (s *WAVESScanner) getBlock(seq int64) (*CommonBlock, error) {
	rb, err := s.wavesRPCClient.GetBlocksBySeq(seq)
	if err != nil {
		return nil, err
	}

	return wavesBlock2CommonBlock(rb)
}

// getBlockAtHeight returns that block at a specific height
func (s *WAVESScanner) getBlockAtHeight(seq int64) (*CommonBlock, error) {
	b, err := s.getBlock(seq)
	return b, err
}

// getNextBlock returns the next block of given hash, return nil if next block does not exist
func (s *WAVESScanner) getNextBlock(seq int64) (*CommonBlock, error) {
	b, err := s.wavesRPCClient.GetBlocksBySeq(seq + 1)
	if err != nil {
		return nil, err
	}
	return wavesBlock2CommonBlock(b)
}

// waitForNextBlock scans for the next block until it is available
func (s *WAVESScanner) waitForNextBlock(block *CommonBlock) (*CommonBlock, error) {
	log := s.log.WithField("blockHash", block.Hash)
	log = log.WithField("blockHeight", block.Height)
	log.Debug("Waiting for the next block")

	for {
		nextBlock, err := s.getNextBlock(int64(block.Height))
		if err != nil {
			if err == ErrEmptyBlock {
				log.WithError(err).Debug("getNextBlock empty")
			} else {
				log.WithError(err).Error("getNextBlock failed")
			}
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
func (s *WAVESScanner) AddScanAddress(addr, coinType string) error {
	return s.Base.GetStorer().AddScanAddress(addr, coinType)
}

// GetScanAddresses returns the deposit addresses that need to scan
func (s *WAVESScanner) GetScanAddresses() ([]string, error) {
	return s.Base.GetStorer().GetScanAddresses(CoinTypeETH)
}

// GetDeposit returns deposit value channel.
func (s *WAVESScanner) GetDeposit() <-chan DepositNote {
	return s.Base.GetDeposit()
}

// WavesClient provides methods for sending coins
type WavesClient struct {
	walletFile string
	changeAddr string
}

// GetTransaction returns transaction by txid
func (c *WavesClient) GetTransaction(txid string) (*model.Transactions, error) {
	transaction, _, err := client.NewTransactionsService().GetTransactionsInfoID(txid)
	return transaction, err
}

// GetBlocks get blocks from RPC
func (c *WavesClient) GetBlocks(start, end int64) (blocks *[]model.Blocks, err error) {
	blocks, _, err = client.NewBlocksService().GetBlocksSeqFromTo(start, end)
	return blocks, err
}

// GetBlocksBySeq get blocks by seq
func (c *WavesClient) GetBlocksBySeq(seq int64) (block *model.Blocks, err error) {
	block, _, err = client.NewBlocksService().GetBlocksAtHeight(seq)
	return block, err
}

// GetLastBlocks get last blocks
func (c *WavesClient) GetLastBlocks() (blocks *model.Blocks, err error) {
	blocks, _, err = client.NewBlocksService().GetBlocksLast()
	return blocks, err
}

// Shutdown the node
func (c *WavesClient) Shutdown() {
}
