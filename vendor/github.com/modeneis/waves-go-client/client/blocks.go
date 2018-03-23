package client

import (
	"fmt"
	"net/http"

	"github.com/dghubble/sling"

	"encoding/json"

	"github.com/modeneis/waves-go-client/model"
	"log"
)

// TransactionsService holds sling instance
type BlocksService struct {
	sling *sling.Sling
}

// NewTransactionsService returns a new AccountService.
func NewBlocksService() *BlocksService {
	return &BlocksService{
		sling: sling.New().Client(nil).Base(MainNET),
	}
}

// NewTransactionsServiceTest returns a new AccountService.
func NewBlocksServiceTest() *BlocksService {
	return &BlocksService{
		sling: sling.New().Base(TestNET),
	}
}

// GetBlocksAtHeight Return block data at the given height
// https://github.com/wavesplatform/Waves/wiki/Waves-Node-REST-API#get-blocksatheight
func (s *BlocksService) GetBlocksAtHeight(Height int64) (*model.Blocks, *http.Response, error) {

	blocks := new(model.Blocks)
	apiError := new(model.APIError)
	path := fmt.Sprintf("/blocks/at/%d", Height)
	res, err := s.sling.New().Get(path).Receive(blocks, apiError)
	if err != nil {
		log.Println("ERROR: GetBlocksAtHeight, ", err)
	}

	return blocks, res, model.FirstError(err, apiError)
}


// GetBlocksSeqFromTo Return block data at the given height range
// https://github.com/wavesplatform/Waves/wiki/Waves-Node-REST-API#get-blocksseqfromto
func (s *BlocksService) GetBlocksSeqFromTo(from, to int64) (*[]model.Blocks, *http.Response, error) {

	blocks := new([]model.Blocks)
	apiError := new(model.APIError)
	path := fmt.Sprintf("/blocks/seq/%d/%d", from,to)
	res, err := s.sling.New().Get(path).Receive(blocks, apiError)
	if err != nil {
		log.Println("ERROR: GetBlocksSeqFromTo, ", err)
	}

	return blocks, res, model.FirstError(err, apiError)
}

// GetBlocksSeqFromTo Return block data at the given height range
// https://github.com/wavesplatform/Waves/wiki/Waves-Node-REST-API#get-blocksseqfromto
func (s *BlocksService) GetBlocksLast() (*model.Blocks, *http.Response, error) {
	blocks := new(model.Blocks)
	apiError := new(model.APIError)
	res, err := s.sling.New().Get("/blocks/last").Receive(blocks, apiError)
	if err != nil {
		log.Println("ERROR: GetBlocksLast, ", err)
	}

	return blocks, res, model.FirstError(err, apiError)
}

func (s *BlocksService) GetBlocksAtHeightDefaultClient(Height int64) (*model.Blocks, *http.Response, error) {
	blocks := model.Blocks{}
	apiError := new(model.APIError)
	path := fmt.Sprintf("/blocks/at/%d", Height)

	req, err := s.sling.New().Get(path).Request()
	if err != nil {
		return &blocks, nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return &blocks, nil, err
	}
	err = json.NewDecoder(res.Body).Decode(&blocks)
	if err != nil {
		return &blocks, nil, err
	}

	return &blocks, res, model.FirstError(err, apiError)
}
