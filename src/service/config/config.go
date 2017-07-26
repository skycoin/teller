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

	Skynode Skynode `json:"skynode"`

	ExchangeRate int64 `json:"exchange_rate"`

	Btcscan   Btcscan   `json:"btc_scan"`
	Btcrpc    Btcrpc    `json:"btc_rpc"`
	SkySender SkySender `json:"sky_sender"`
}

// Btcscan config for scanner
type Btcscan struct {
	CheckPeriod       time.Duration `json:"check_period"`
	DepositBufferSize uint32        `json:"deposit_buffer_size"`
}

// Skynode represents the skycoin node related config
type Skynode struct {
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

	cfg.PingTimeout = cfg.PingTimeout * time.Second
	cfg.PongTimeout = cfg.PongTimeout * time.Second
	cfg.DialTimeout = cfg.DialTimeout * time.Second
	cfg.ReconnectTime = cfg.ReconnectTime * time.Second

	return &cfg, nil
}

// ExchangeRate represents the exchange rate, it has two field, Time and Rate
// Time should be in the form of 2017-04-30 00:00:00
type ExchangeRate struct {
	Date string  `json:"date"`
	Rate float64 `json:"rate"`
}

// Btcrpc config for btcrpc
type Btcrpc struct {
	Server string `json:"server"`
	User   string `json:"user"`
	Pass   string `json:"pass"`
	Cert   string `json:"cert"`
}

// SkySender config for skycoin sender
type SkySender struct {
	ReqBuffSize uint32 `json:"request_buffer_size"`
}
