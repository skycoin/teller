# Teller

[![Build Status](https://travis-ci.org/skycoin/teller.svg?branch=master)](https://travis-ci.org/skycoin/teller)

<!-- MarkdownTOC autolink="true" bracket="round" depth="2" -->

- [Releases & Branches](#releases--branches)
- [Setup project](#setup-project)
    - [Prerequisites](#prerequisites)
    - [Summary of setup for development without btcd or skycoind](#summary-of-setup-for-development-without-btcd-or-skycoind)
    - [Start teller-proxy](#start-teller-proxy)
    - [Start teller](#start-teller)
- [Service apis](#service-apis)
    - [Bind](#bind)
    - [Status](#status)
    - [Config](#config)
- [Code linting](#code-linting)
- [Run tests](#run-tests)
- [Database structure](#database-structure)

<!-- /MarkdownTOC -->


## Releases & Branches

Last stable release is in `master` branch, which will match the latest tagged release.

Pull requests should target the `develop` branch, which is the default branch.

If you clone, make sure you `git checkout master` to get the latest stable release.

When a new release is published, `develop` will be merged to `master` then tagged.

## Setup project

### Prerequisites

* Have go1.8+ installed
* Have `GOPATH` env set
* Have btcd started
* Have skycoin node started

### Summary of setup for development without btcd or skycoind

```sh
# Generate btc_addresses.json file. 'foobar' is an arbitrary seed, and 10 is an arbitrary number of addresses to generate
go run cmd/tool/tool.go -json newbtcaddress foobar 10 > /tmp/btc_addresses.json

# Run proxy, a pubkey will be printed to stdout, copy it
go run cmd/proxy/proxy.go

# In a new terminal, run teller in dummy mode, provide pubkey from proxy stdout, point addresses to addr file
cd cmd/teller/
go run teller.go -proxy-pubkey=<proxy pubkey> -dummy -btc-addrs=/tmp/btc_addresses.json
```

Proxy API is available on `localhost:7071`. API has two methods, `/api/bind` and `/api/status`, with one query arg `skyaddr`, e.g.:

```sh
wget http://localhost:7071/api/bind?skyaddr=<skycoin addr>
```

### Start teller-proxy

*Note: the proxy must be run from the repo root, in order to serve static content from `./web/dist`*

```sh
make proxy
```

Once the proxy starts, it will show a `pubkey` value in the log.
Use this for the `-proxy-pubkey` argument in `teller`.

```sh
18:28:49 proxy.go:33: Pubkey: 03583bf0a6cbe7048023be5475aca693e192b4b5570bcc883c50f7d96f0a996eda
```

### Start teller

Install `skycoin-cli`

```sh
make install-skycoin-cli
```

add pregenerated bitcoin deposit address list in `btc_addresses.json`.

```sh
{
    "btc_addresses": [
        "1PZ63K3G4gZP6A6E2TTbBwxT5bFQGL2TLB",
        "14FG8vQnmK6B7YbLSr6uC5wfGY78JFNCYg",
        "17mMWfVWq3pSwz7BixNmfce5nxaD73gRjh",
        "1Bmp9Kv9vcbjNKfdxCrmL1Ve5n7gvkDoNp"
    ]
}
```

use `tool` to pregenerate bitcoin address list:

```sh
cd cmd/tool
go run tool.go -json newbtcaddress $seed $num
```

Example:

```sh
go run tool.go -json newbtcaddress 12323 3

2Q5sR1EgTWesxX9853AnNpbBS1grEY1JXn3
2Q8y3vVAqY8Q3paxS7Fz4biy1RUTY5XQuzb
216WfF5EcvpVk6ypSRP3Lg9BxqpUrgBJBco
```

generate json file Example:

```sh
go run tool.go -json newbtcaddress 12323 3 > new_btc_addresses.json
```


teller's config is managed in `config.json`, need to set the `wallet_path`
in `skynode` field to an absolute path of skycoin wallet file, and set up the `btcd`
config in `btc_rpc` field, including server address, username, password and
absolute path to the cert file.

config.json:

```json
{
    "http_address": "127.0.0.1:7070",
    "reconnect_time": 5,
    "dial_timeout": 5,
    "ping_timeout": 5,
    "pong_timeout": 10,
    "exchange_rate": 500,
    "skynode": {
        "rpc_address": "127.0.0.1:6430",
        "wallet_path": "absolute path to the wallet file"
    },
    "btc_scan": {
        "check_period": 20,
        "deposit_buffer_size": 1024
    },
    "btc_rpc": {
        "server": "127.0.0.1:8334",
        "user": "",
        "pass": "",
        "cert": "absolute path to rpc cert file"
    },
    "sky_sender": {
        "request_buffer_size": 1024
    }
}
```

run teller service

```sh
go run teller.go -proxy-pubkey=$the_pubkey_of_proxy
```

## Service apis

The HTTP API service is provided by the proxy and serve on port 7071 by default.

The API returns JSON for all 200 OK responses.

If the API returns a non-200 response, the response body is the error message, in plain text (not JSON).

### Bind

```sh
Method: POST
Accept: application/json
Content-Type: application/json
URI: /api/bind
Request Body: {
    "skyaddr": "...",
    "coin_type": "BTC"
}
```

Binds a skycoin address to a BTC address. A skycoin address can be bound to
multiple BTC addresses.  The default maximum number of bound addresses is 5.

Coin type specifies which coin deposit address type to generate.
Options are: BTC [TODO: support more coin types].

Example:

```sh
curl -H  -X POST "Content-Type: application/json" -d '{"skyaddr":"...","coin_type":"BTC"}' http://localhost:7071/api/bind
```

Response:

```json
{
    "deposit_address": "1Bmp9Kv9vcbjNKfdxCrmL1Ve5n7gvkDoNp",
    "coin_type": "BTC",
}
```

### Status

```sh
Method: GET
Content-Type: application/json
URI: /api/status
Query Args: skyaddr
```

Returns statuses of a skycoin address.

Since a single skycoin address can be bound to multiple BTC addresses the result is in an array.
The default maximum number of BTC addresses per skycoin address is 5.

We cannot return the BTC address for security reasons so they are numbered and timestamped instead.

Possible statuses are:

* `waiting_deposit` - Skycoin address is bound, no deposit seen on BTC address yet
* `waiting_send` - BTC deposit detected, waiting to send skycoin out
* `waiting_confirm` - Skycoin sent out, waiting to confirm the skycoin transaction
* `done` - Skycoin transaction confirmed

Example:

```sh
curl http://localhost:7071/api/status?skyaddr=t5apgjk4LvV9PQareTPzWkE88o1G5A55FW
```

Response:

```json
{
    "statuses": [
        {
            "seq": 1,
            "updated_at": 1501137828,
            "status": "done"
        },
        {
            "seq": 2,
            "updated_at": 1501128062,
            "status": "waiting_deposit"
        },
        {
            "seq": 3,
            "updated_at": 1501128063,
            "status": "waiting_deposit"
        },
    ]
}
```

### Config

```sh
Method: GET
Content-Type: application/json
URI: /api/config
```

Returns teller configuration.

Example:

```sh
curl http://localhost:7071/api/config
```

Response:

```json
{
    "enabled": true,
    "btc_confirmations_required": 1,
    "max_bound_btc_addrs": 5,
    "sky_btc_exchange_rate": "123.000000"
}
```

## Code linting

```sh
make install-linters
make lint
```

## Run tests

```sh
make test
```

## Database structure

```
Bucket: used_btc_address
File: btcaddrs/store.go

Maps: `btcaddr -> ""`
Note: Marks a btc address as used
```

```
Bucket: exchange_meta
File: exchange/store.go

Note: unused
```

```
Bucket: deposit_info
File: exchange/store.go

Maps: btcTx[%tx:%n] -> exchange.DepositInfo
Note: Maps a btc txid:seq to exchange.DepositInfo struct
```

```
Bucket: bind_address
File: exchange/store.go

Maps: btcaddr -> skyaddr
Note: Maps a btc addr to a sky addr
```

```
Bucket: sky_deposit_seqs_index
File: exchange/store.go

Maps: skyaddr -> [btcaddrs]
Note: Maps a sky addr to multiple btc addrs
```

```
Bucket: btc_txs
File: exchange/store.go

Maps: btcaddr -> [txs]
Note: Maps a btcaddr to multiple btc txns
```

```
Bucket: scan_meta
File: scanner/store.go

Maps: "deposit_addresses" -> [btcaddrs]
Note: Saves list of btc addresss being scanned

Maps: "dv_index_list" -> [btcTx[%tx:%n]][json]
Note: Saves list of btc txid:seq (as JSON)
```

```
Bucket: deposit_value
File: scanner/store.go

Maps: btcTx[%tx:%n] -> scanner.Deposit
Note: Maps a btc txid:seq to scanner.Deposit struct
```
