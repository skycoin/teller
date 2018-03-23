package client

import (
	"fmt"
	"net/http"

	"github.com/dghubble/sling"

	"github.com/modeneis/waves-go-client/model"
)

// TransactionsService holds sling instance
type TransactionsService struct {
	sling *sling.Sling
}

// NewTransactionsService returns a new AccountService.
func NewTransactionsService() *TransactionsService {
	return &TransactionsService{
		sling: sling.New().Base(MainNET),
	}
}

// NewTransactionsServiceTest returns a new AccountService.
func NewTransactionsServiceTest() *TransactionsService {
	return &TransactionsService{
		sling: sling.New().Base(TestNET),
	}
}

// GetTransactionsInfoID Return transaction data by transaction ID
// https://github.com/wavesplatform/Waves/wiki/Waves-Node-REST-API#get-transactionsinfoid
func (s *TransactionsService) GetTransactionsInfoID(ID string) (*model.Transactions, *http.Response, error) {
	transaction := new(model.Transactions)
	apiError := new(model.APIError)
	path := fmt.Sprintf("/transactions/info/%s", ID)
	resp, err := s.sling.New().Get(path).Receive(transaction, apiError)
	return transaction, resp, model.FirstError(err, apiError)
}
