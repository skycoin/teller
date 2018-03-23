package model

// Transactions is transaction response body
type Blocks struct {
	Version          int            `json:"version"`
	Timestamp        int64          `json:"timestamp"`
	Reference        string         `json:"reference"`
	NxtCconsensus    NxtCconsensus  `json:"nxt-consensus"`
	Features         []int       `json:"features"`
	Generator        string         `json:"generator"`
	Signature        string         `json:"signature"`
	Blocksize        int            `json:"blocksize"`
	TransactionCount int            `json:"transactionCount"`
	Fee              int64          `json:"fee"`
	Transactions     []Transactions `json:"transactions"`
	Height           int64          `json:"height"`
}

type Order struct {
	Type             int       `json:"type"`
	ID               string    `json:"id"`
	SenderPublicKey  string    `json:"senderPublicKey"`
	MatcherPublicKey string    `json:"matcherPublicKey,omitempty"`
	AssetPair        AssetPair `json:"assetPair,omitempty"`

	OrderType  string `json:"orderType,omitempty"`
	Price      int    `json:"price,omitempty"`
	Expiration int    `json:"expiration,omitempty"`
	MatcherFee int    `json:"matcherFee,omitempty"`
	Signature  string `json:"signature"`

	Amount    int64 `json:"amount"`
	Timestamp int64 `json:"timestamp"`
}

type AssetPair struct {
	AmountAsset string `json:"amountAsset"`
	PriceAsset  string `json:"priceAsset"`
}
type NxtCconsensus struct {
	BaseTarget          int64  `json:"base-target"`
	GenerationSignature string `json:"generation-signature"`
}
