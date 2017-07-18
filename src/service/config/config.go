// Package config is used to records the service configurations
package config

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"time"
)

const (
	// RateTimeLayout represents the rate time layout
	RateTimeLayout = "2006-01-02 15:04:05"
)

// Config represents the configuration root
type Config struct {
	ProxyAddress  string        `json:"proxy_address"`
	ReconnectTime time.Duration `json:"reconnect_time"`
	DialTimeout   time.Duration `json:"dial_timeout"`
	PingTimeout   time.Duration `json:"ping_timeout"`
	PongTimeout   time.Duration `json:"pong_timeout"`

	Monitor Monitor `json:"monitor"`
	Node    Node    `json:"node"`

	ExchangeRate []ExchangeRate `json:"exchange_rate"`

	DepositCoin string `json:"deposit_coin"`
	ICOCoin     string `json:"ico_coin"`
}

// Monitor represents  monitor related config
type Monitor struct {
	CheckPeriod time.Duration `json:"check_period"`
}

// Node represents the node related config
type Node struct {
	RPCAddress string `json:"rpc_address"`
	WalletPath string `json:"wallet_path"`
}

// New loads configuration from the given filepath,
func New(path string) (*Config, error) {
	var cfg Config
	v, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := json.NewDecoder(bytes.NewReader(v)).Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// ExchangeRate represents the exchange rate, it has two field, Time and Rate
// Time should be in the form of 2017-04-30 00:00:00
type ExchangeRate struct {
	Date string  `json:"date"`
	Rate float64 `json:"rate"`
}
