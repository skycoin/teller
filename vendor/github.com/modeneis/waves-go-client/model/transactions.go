package model

// Transactions is transaction response body
type Transactions struct {
	Type            int    `json:"type"`
	ID              string `json:"id"`
	Sender          string `json:"sender"`
	SenderPublicKey string `json:"senderPublicKey"`
	Recipient       string `json:"recipient"`
	AssetID         string `json:"assetId"`
	Amount          int64  `json:"amount"`
	FeeAsset        string `json:"feeAsset"`
	Fee             int64  `json:"fee"`
	Timestamp       int64  `json:"timestamp"`
	Attachment      string `json:"attachment"`
	Signature       string `json:"signature"`
	Height          int64  `json:"height"`

	// Blocks
	OrderType  AssetPair `json:"orderType,omitempty"`
	Price      int       `json:"price,omitempty"`
	Expiration int       `json:"expiration,omitempty"`
	MatcherFee int       `json:"matcherFee,omitempty"`

	Order1 Order `json:"order1,omitempty"`
	Order2 Order `json:"order2,omitempty"`

	BuyMatcherFee  int64 `json:"buyMatcherFee,omitempty"`
	SellMatcherFee int64 `json:"sellMatcherFee,omitempty"`
}
