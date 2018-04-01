![teller logo](https://user-images.githubusercontent.com/26845312/32426949-3682cd18-c283-11e7-9067-55084310064d.png)

# Teller

[![Build Status](https://travis-ci.org/skycoin/teller.svg?branch=master)](https://travis-ci.org/skycoin/teller)
[![GoDoc](https://godoc.org/github.com/skycoin/teller?status.svg)](https://godoc.org/github.com/skycoin/teller)
[![Go Report Card](https://goreportcard.com/badge/github.com/skycoin/teller)](https://goreportcard.com/report/github.com/skycoin/teller)
[![Docker Pulls](https://img.shields.io/docker/pulls/skycoin/teller.svg?maxAge=604800)](https://hub.docker.com/r/skycoin/teller/)

<!-- MarkdownTOC autolink="true" bracket="round" depth="5" -->

- [Releases & Branches](#releases--branches)
- [Setup project](#setup-project)
    - [Prerequisites](#prerequisites)
    - [Configure teller](#configure-teller)
    - [Running teller without btcd, geth or skyd](#running-teller-without-btcd-geth-or-skyd)
    - [Running teller with Docker](#running-teller-with-docker)
    - [Generating deposit addresses](#generating-deposit-addresses)
        - [Generate BTC addresses](#generate-btc-addresses)
        - [Generate ETH addresses](#generate-eth-addresses)
    - [Setup skycoin hot wallet](#setup-skycoin-hot-wallet)
    - [Run teller](#run-teller)
    - [Setup skycoin node](#setup-skycoin-node)
    - [Setup btcd](#setup-btcd)
        - [Configure btcd](#configure-btcd)
        - [Obtain btcd RPC certificate](#obtain-btcd-rpc-certificate)
        - [BTC scanning notes](#btc-scanning-notes)
    - [Using a reverse proxy to expose teller](#using-a-reverse-proxy-to-expose-teller)
    - [Setup geth](#setup-geth)
        - [Configure geth](#configure-geth)
- [Public API](#public-api)
    - [Bind](#bind)
    - [Status](#status)
    - [Config](#config)
    - [Health](#health)
- [Dummy Scanner and Sender API](#dummy-scanner-and-sender-api)
    - [Scanner](#scanner)
        - [Deposit](#deposit)
    - [Sender](#sender)
        - [Broadcasts](#broadcasts)
        - [Confirm](#confirm)
- [Monitoring API](#monitoring-api)
    - [Deposit Address Usage](#deposit-address-usage)
    - [Deposits By Status](#deposits-by-status)
    - [Deposit Errors](#deposit-errors)
    - [Accounting](#accounting)
    - [Backup](#backup)
- [Code linting](#code-linting)
- [Run tests](#run-tests)
- [Database structure](#database-structure)
- [Frontend development](#frontend-development)
- [Integration testing](#integration-testing)
- [Monitoring logs](#monitoring-logs)
- [Logrotate integration](#logrotate-integration)
- [Passthrough notes](#passthrough-notes)

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
* [Setup skycoin node](#setup-skycoin-node)
* [Setup btcd](#setup-btcd)
* [Setup geth](#setup-geth)

### Configure teller

The config file can be specified with `-c` or `--config` from the command line.

Otherwise, the config file is loaded from the application data directory
(defaulted to `$HOME/.teller-skycoin`), or the local working directory.

The application data directory can be changed from the command line with
`-d` or `--dir`.

The config file uses the [toml](https://github.com/toml-lang/toml) format.

Teller's default config is `config.toml`. However, you should not edit this
file. It is an example. Copy this config file and edit it for your needs,
then use the `-c` or `--config` flag to load your custom config.

Description of the config file:

* `debug` [bool]: Enable debug logging.
* `profile` [bool]: Enable gops profiler.
* `logfile` [string]: Log file.  It can be an absolute path or be relative to the working directory.
* `dbfile` [string]: Database file, saved inside the `~/.teller-skycoin` folder. Do not use a path.
* `pidfile` [string]: PID file, optional. Use for daemon integration.
* `btc_addresses` [string]: Filepath of the BTC addresses file. See [generate BTC addresses](#generate-btc-addresses).
* `eth_addresses` [string]: Filepath of the ETH addresses file. See [generate ETH addresses](#generate-eth-addresses).
* `teller.max_bound_addrs` [int]: Maximum number addresses allowed to bind per skycoin address.
* `teller.bind_enabled` [bool]: Disable this to prevent binding of new addresses
* `sky_rpc.address` [string]: Host address of the skycoin node. See [setup skycoin node](#setup-skycoin-node).
* `btc_rpc.server` [string]: Host address of the btcd node.
* `btc_rpc.user` [string]: btcd RPC username.
* `btc_rpc.pass` [string]: btcd RPC password.
* `btc_rpc.cert` [string]: btcd RPC certificate file. See [setup btcd](#setup-btcd)
* `btc_rpc.cert` [bool]: Use a websocket connection instead of HTTP POST requests.
* `btc_scanner.scan_period` [duration]: How often to scan for blocks.
* `btc_scanner.initial_scan_height` [int]: Begin scanning from this BTC blockchain height.
* `btc_scanner.confirmations_required` [int]: Number of confirmations required before sending skycoins for a BTC deposit.
* `eth_rpc.server` [string]: Host address of the geth node.
* `eth_rpc.port` [string]: Host port of the geth node.
* `eth_scanner.scan_period` [duration]: How often to scan for ethereum blocks.
* `eth_scanner.initial_scan_height` [int]: Begin scanning from this ETH blockchain height.
* `eth_scanner.confirmations_required` [int]: Number of confirmations required before sending skycoins for a ETH deposit.
* `sky_exchanger.max_decimals` [int]: Number of decimal places to truncate SKY to.
* `sky_exchanger.sky_btc_exchange_rate` [string]: How much SKY to send per BTC. This can be written as an integer, float, or a rational fraction.
* `sky_exchanger.sky_eth_exchange_rate` [string]: How much SKY to send per ETH. This can be written as an integer, float, or a rational fraction.
* `sky_exchanger.wallet` [string]: Filepath of the skycoin hot wallet. See [setup skycoin hot wallet](#setup-skycoin-hot-wallet).
* `sky_exchanger.tx_confirmation_check_wait` [duration]: How often to check for a sent skycoin transaction's confirmation.
* `sky_exchanger.send_enabled` [bool]: Disable this to prevent sending of coins (all other processing functions normally, e.g.. deposits are received)
* `sky_exchanger.buy_method` [string]: Options are "direct" or "passthrough". "direct" will send directly from the wallet. "passthrough" will purchase from an exchange before sending from the wallet.
* `sky_exchanger.exchange_client.key` [string]: C2CX API key.  Required if `sky_exchanger.buy_method` is "passthrough".
* `sky_exchanger.exchange_client.secret` [string]: C2CX API secret key.  Required if `sky_exchanger.buy_method` is "passthrough".
* `sky_exchanger.exchange_client.request_failure_wait` [duration]: How long to wait after a request failure to C2CX.
* `sky_exchanger.exchange_client.ratelimit_wait` [duration]: How long to wait after being ratelimited by the C2CX API.
* `sky_exchanger.exchange_client.check_order_wait` [duration]: How long to wait between requests to check order status on C2CX.
* `sky_exchanger.exchange_client.btc_minimum_volume` [decimal]: Minimum BTC volume allowed for a deposit. C2CX's minimum is variable, this should be set to some higher arbitrary value to avoid making a failed order.
* `web.behind_proxy` [bool]: Set true if running behind a proxy.
* `web.static_dir` [string]: Location of static web assets.
* `web.throttle_max` [int]: Maximum number of API requests allowed per `web.throttle_duration`.
* `web.throttle_duration` [int]: Duration of throttling, pairs with `web.throttle_max`.
* `web.http_addr` [string]: Host address to expose the HTTP listener on.
* `web.https_addr` [string] Host address to expose the HTTPS listener on.
* `web.auto_tls_host` [string]: Hostname/domain to install an automatic HTTPS certificate for, using Let's Encrypt.
* `web.tls_cert` [string]: Filepath to TLS certificate. Cannot be used with `web.auto_tls_host`.
* `web.tls_key` [string]: Filepath to TLS key. Cannot be used with `web.auto_tls_host`.
* `web.cors_allowed` [array of strings]: List of domains to allow for CORS requests. To allow a desktop wallet to make requests, add the desktop wallet's `127.0.0.1:port` interface.
* `admin_panel.host` [string] Host address of the admin panel.
* `dummy.sender` [bool]: Use a fake SKY sender (See ["dummy mode"](#summary-of-setup-for-development-without-btcd-or-skycoind)).
* `dummy.scanner` [bool]: Use a fake BTC scanner (See ["dummy mode"](#summary-of-setup-for-development-without-btcd-or-skycoind)).
* `dummy.http_addr` [bool]: Host address for the dummy scanner and sender API.

### Running teller without btcd, geth or skyd

Teller can be run in "dummy mode". It will ignore btcd, geth and skycoind.
It will still provide addresses via `/api/bind` and report status with `/api/status`.
but it will not process any real deposits or send real skycoins.

See the [dummy API](#dummy) for controlling the fake deposits and sends.

### Running teller with Docker

Teller can be run with Docker. Update the `config.toml`, to send the logs to
stdout, make sure the logfile key is set to an empty string and that every
addesses listen to `0.0.0.0` instead of `127.0.0.1`:

```toml
logfile = ""
dbfile = "/data"

[web]
http_addr = "0.0.0.0:7071"

[admin_panel]
host = "0.0.0.0:7711"

[dummy]
http_addr = "0.0.0.0:4121"
```

Then run the following command to start teller:

```sh
docker volume create teller-data
docker run -ti --rm \
  -p 4121:4121 \
  -p 7071:7071 \
  -p 7711:7711 \
  -v $PWD/config.toml:/usr/local/teller/config.toml \
  -v $PWD/btc_addresses.json:/usr/local/teller/btc_addresses.json \
  -v $PWD/eth_addresses.json:/usr/local/teller/eth_addresses.json \
  -v teller-data:/data
  skycoin/teller
```

Access the dashboard: [http://localhost:7071](http://localhost:7071).

### Generating deposit addresses

Deposit address files can be either a JSON file or a text file.

Text files are a newline separate list of addresses.  Empty lines and lines beginning with `#` are ignored.

It will be considered a JSON file if the filename ends with `.json`, otherwise it will be parsed as a text file.

#### Generate BTC addresses

Use `tool` to pregenerate a list of bitcoin addresses parseable by teller.

Generating the text file:

```sh
go run cmd/tool/tool.go $seed $num > btc_addresses.txt
```

Generating the JSON file:

```sh
go run cmd/tool/tool.go -json newbtcaddress $seed $num > btc_addresses.json
```

JSON file format:

```json
{
    "btc_addresses": [...]
}
```

#### Generate ETH addresses

```
Creating a new account

geth account new
Creates a new account and prints the address.

On the console, command (`geth console`) use:

> personal.newAccount("passphrase")
The account is saved in encrypted format. You must remember this passphrase to unlock your account in the future.

For non-interactive use the passphrase can be specified with the --password flag:

geth --password <passwordfile> account new
Note, this is meant to be used for testing only, it is a bad idea to save your password to file or expose in any other way.
```

then put those address into an `eth_addresses.txt` or `eth_addresses.json` file.

### Setup skycoin hot wallet

Use the skycoin client or CLI to create a wallet. Copy this wallet file to
the machine that teller is running on, and specify its path in the config file
under `sky_exchanger` section, `wallet` parameter.

Load this wallet with a sufficient balance for your OTC.

If the balance is insufficient, the skycoin sender will repeatedly try to send
coins for a deposit until the balance becomes sufficient.

### Run teller

*Note: teller must be run from the repo root, in order to serve static content from `./web/dist`*

```sh
make teller
```

### Setup skycoin node

See https://github.com/skycoin/skycoin#installation

Use the skycoin daemon's RPC interface address as the value of `sky_rpc.address`
in the config.  The default is already configured to `127.0.0.1:6430` so there
is no need to set this value if the skycoin daemon is run with defaults.

*Note: skycoin daemon RPC does not use encryption so only run it on the same machine
as teller or on a secure LAN*

### Setup btcd

Follow the instructions from the btcd README to install btcd:
https://github.com/btcsuite/btcd#linuxbsdmacosxposix---build-from-source

Update your `PATH` environment to include `$GOPATH/bin`. The default `$GOPATH` is `~/go`.

Then, run `btcd`:

```sh
btcd
```

This creates a folder `~/.btcd`.

#### Configure btcd

btcd includes a reference conf file: https://github.com/btcsuite/btcd/blob/master/sample-btcd.conf

In `~/.btcd/btcd.conf`, edit the following values:

* `txindex` - set this to `1`.
* `rpcuser` - use a long, random, secure string for this value. Set this as the value of `btc_rpc.user` in the teller conf.
* `rpcpass` - use a long, random, secure string for this value. Set this as the value of `btc_rpc.pass` in the teller conf.

If you are not running btcd on the same machine as teller, you need to change
the `rpclisten` value too.  It will need to listen on a public interface.
Refer to the sample config link above for instructions on how to configure this value.

Now, run `btcd` again:

```sh
btcd
```

Once the RPC interface is enabled (by setting `rpcuser` and `rpcpass`),
`btcd` will generate `~/.btcd/rpc.cert` and `~/.btcd/rpc.key`.

#### Obtain btcd RPC certificate

Use the path of the `~/.btcd/rpc.cert` file as the value of `btc_rpc.cert` in teller's config.
If teller is running on a different machine, you will need to move it there first.
Do not copy `~/.btcd/rpc.key`, this is a secret key and is not needed by teller.

#### BTC scanning notes

Multisig outputs are not supported. If an output has multiple addresses assigned to it, it is ignored and not considered as a valid deposit.

### Using a reverse proxy to expose teller

SSH reverse proxy method:

```sh
# open reverse proxy. -f runs it in the background as daemon, if you don't want this, remove it
ssh -fnNT -R '*:7071:localhost:7071' user@server
# forward port 80 traffic to localhost:7071, since can't bind reverse proxy to port 80 directly
# Change enp2s0 to your interface name. Use ifconfig to see your interfaces.
sudo iptables -t nat -A PREROUTING -i enp2s0 -p tcp --dport 80 -j REDIRECT --to-port 7071
```

The iptables rule needs to be set permanently. One method for doing this:


```sh
sudo apt-get install iptables-persistent
# it will ask if you want to save the current rules, don't save if you have ufw running, it will conflict
# edit /etc/iptables/rules.v4 and /etc/iptables/rules.v6, add the port redirect line
sudo echo "iptables -t nat -A PREROUTING -i enp2s0 -p tcp --dport 80 -j REDIRECT --to-port 7071" >> /etc/iptables/rules.v4
sudo echo "iptables -t nat -A PREROUTING -i enp2s0 -p tcp --dport 80 -j REDIRECT --to-port 7071" >> /etc/iptables/rules.v6
```

These rules need to be duplicated for another port (e.g. 7072) for the HTTPS listener, when exposing HTTPS.

### Setup geth

Follow the instructions from the geth wiki to install geth:
https://github.com/ethereum/go-ethereum/wiki/Building-Ethereum

Building geth requires both a Go (version 1.7 or later) and a C compiler.
Once the dependencies are installed, run

```sh
cd go-ethereum
make geth
```

The `geth` binary is placed in `build/bin/geth`. Add geth to your `PATH` so that
it can be invoked.

#### Configure geth

Specify the following values:

* `--datadir` - set this to directory that store ethereum block chain data.
* `--rpc` - Enable the HTTP-RPC server
* `--rpcaddr` - HTTP-RPC server listening interface (default: "localhost") Set this as the value of `eth_rpc.server` in the teller conf.
* `--rpcport`- HTTP-RPC server listening port (default: 8545) Set this as the value of `eth_rpc.port` in the teller conf.

Please note, offering an API over the HTTP (rpc) or WebSocket (ws) interfaces will give
everyone access to the APIs who can access this interface (DApps, browser tabs, etc). Be
careful which APIs you enable. By default Geth enables all APIs over the IPC (ipc) interface
and only the db, eth, net and web3 APIs over the HTTP and WebSocket interfaces.

See [Geth API docs](https://github.com/ethereum/go-ethereum/wiki/Management-APIs) for more information.

>Note: You can using a reverse proxy to expose geth rpc port such as [Using a reverse proxy to expose teller](#using-a-reverse-proxy-to-expose-teller)

Now, run `geth`:

```sh
geth --datadir=xxx
```

## Public API

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

Binds a skycoin address to a BTC/ETH address. A skycoin address can be bound to
multiple BTC/ETH addresses. The default maximum number of bound addresses is 5.

Coin type specifies which coin deposit address type to generate.
Options are: BTC/ETH [TODO: support more coin types].

"buy_method" in the response, indicates the purchasing mode.
"direct" buy method is a fixed-price purchase directly from the wallet.
"passthrough" but method is a variable-price purchase through an exchange.

Returns `403 Forbidden` if `teller.bind_enabled` is `false`.

Example:

```sh
curl -H  -X POST "Content-Type: application/json" -d '{"skyaddr":"...","coin_type":"BTC"}' http://localhost:7071/api/bind
```

Response:

```json
{
    "deposit_address": "1Bmp9Kv9vcbjNKfdxCrmL1Ve5n7gvkDoNp",
    "coin_type": "BTC",
    "buy_method": "direct"
}
```
ETH example:
```sh
curl -H  -X POST "Content-Type: application/json" -d '{"skyaddr":"...","coin_type":"ETH"}' http://localhost:7071/api/bind
```

Response:

```json
{
    "deposit_address": "0x392cded14b8f12cb6cbb1c7922810f4fbd80c3f6",
    "coin_type": "ETH",
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

Since a single skycoin address can be bound to multiple BTC/ETH addresses the result is in an array.
The default maximum number of BTC/ETH addresses per skycoin address is 5.

We cannot return the BTC/ETH address for security reasons so they are numbered and timestamped instead.

Possible statuses are:

* `waiting_deposit` - Skycoin address is bound, no deposit seen on BTC/ETH address yet
* `waiting_send` - BTC/ETH deposit detected, waiting to send skycoin out
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

If `"enabled"` is `false`, `/api/bind` will return `403 Forbidden`. `/api/status` will still work.

`"buy_method"` is either "direct" or "passthrough".

If `"buy_method"` is "passthrough", then the `"btc_minimum_volume"` is the minimum amount of BTC that a
user should send.

Example:

```sh
curl http://localhost:7071/api/config
```

Response:

```json
{
    "enabled": true,
    "buy_method": "passthrough",
    "max_bound_addrs": 5,
    "max_decimals": 3,
    "deposits":
    {
        "btc":
        {
            "enabled": true,
            "confirmations_required": 1,
            "fixed_exchange_rate": "123.000000",
            "passthrough_minimum_volume": "0.005"
        },
        "eth":
        {
            "enabled": false,
            "confirmations_required": 5,
            "fixed_exchange_rate": "30.000000",
            "passthrough_minimum_volume": "1.5"
        }
    }
}
```

### Health

```sh
Method: GET
Content-Type: application/json
URI: /api/health
```

Return teller's status.

Field `exchange_status`:
The exchanger's status includes last error seen while trying to send SKY, or nil if no last error was seen.
Use this to detect if the OTC is temporarily offline, sold out or there is a problem with OTC passthrough.

The balance of the OTC wallet is included in the response.  The wallet may still be considered "sold out" even
if there is a balance remaining. The wallet is considered "sold out" when there are not enough coins in the wallet
to satisfy the current purchase, so it is unlikely for the wallet to ever reach exactly 0.  For example, if there
are 100 coins in the wallet and someone attempts to purchase 200 coins, it will be considered "sold out".
In this case, the "error" field will be set to some message string, and the balance will say "100.000000".

Example:

```sh
curl http://localhost:7071/api/health
```

Response:

```json
{
    "status": "OK",
    "application": "teller",
    "version": "6720f7c5183e01090e657cda88e4ca962969c874",
    "uptime": "5m32s",
    "exchange": {
        "buy_method": "direct",
        "processor_error": "",
        "sender_error": "",
        "deposit_error_count": 4,
        "balance": {
            "coins": "100.000000",
            "hours": "100"
        }
    }
}
```

```json
{
    "status": "OK",
    "application": "teller",
    "version": "6720f7c5183e01090e657cda88e4ca962969c874",
    "uptime": "5m32s",
    "exchange": {
        "buy_method": "passthrough",
        "processor_error": "",
        "sender_error": "no unspents to spend",
        "deposit_error_count": 0,
        "balance": {
            "coins": "0.000000",
            "hours": "0"
        }
    }
}
```


Possible statuses are:
TODO

## Dummy Scanner and Sender API

A dummy scanner and sender API is available over `dummy.http_addr` if
`dummy.scanner` or `dummy.sender` are enabled.

### Scanner

#### Deposit

```sh
Method: GET, POST
URI: /dummy/scanner/deposit
```

Adds a deposit to the scanner.

Example:

```sh
curl http://localhost:4121/dummy/scanner/deposit?addr=1PZ63K3G4gZP6A6E2TTbBwxT5bFQGL2TLB&value=100000000&height=494713&tx=edb29a9b561a8d6a6118eb1f724c87f853bf471d7e4f0e9ccb9e1d340235687b&n=0
```

### Sender

#### Broadcasts

```sh
Method: GET
URI: /dummy/sender/broadcasts
```

Lists broadcasted skycoin transactions.  Note: the dummy sender keeps its records
in memory, if teller is restarted, the list of broadcasted transactions is reset.

Example:

```sh
curl http://localhost:4121/dummy/sender/broadcasts
```

Response:

```json
[
    {
        "txid": "4fc9743b04c2e3f5e467cde38c0872e3e3ad9ec05d59081ad1a8bd88045635de",
        "outputs": [
            {
                "address": "vpfRRmfPU11HSZjKroskGVnHg4CJ5zJ4ax",
                "coins": "500.000000"
            }
        ],
        "confirmed": false
    }
]
```

#### Confirm

```sh
Method: GET, POST
URI: /dummy/sender/confirm
```

Confirms a broadcasted transaction.

Example:

```sh
curl http://localhost:4121/dummy/sender/confirm?txid=4fc9743b04c2e3f5e467cde38c0872e3e3ad9ec05d59081ad1a8bd88045635de
```

## Monitoring API

Admin monitoring APIs are exposed on `http://localhost:7711` by default.
This is configured in the `admin_panel` section of the config file.

### Deposit Address Usage

```sh
Method: GET
URI: /api/deposit-addresses
```

Returns information about the deposit address list.

Example:

```sh
curl http://localhost:7711/api/deposit-addresses
```

Response:

```json
{
    "BTC": {
        "remaining_addresses": 4,
        "scanning_addresses": ["1PZ63K3G4gZP6A6E2TTbBwxT5bFQGL2TLB"],
        "scanning_enabled": true,
        "address_manager_enabled": true
    },
    "ETH": {
        "remaining_addresses": 5,
        "scanning_addresses": [],
        "scanning_enabled": false,
        "address_manager_enabled": true
    },
    "SKY": {
        "remaining_addresses": 0,
        "scanning_addresses": [],
        "scanning_enabled": false,
        "address_manager_enabled": false
    }
}
```

### Deposits By Status

```sh
Method: GET
URI: /api/deposits
Args:
    status - Optional, one of "waiting_deposit", "waiting_send", "waiting_confirm", "done", "waiting_decide", "waiting_passthrough", "waiting_passthrough_order_complete"
```

Returns all deposits with a given status, or all deposits if no status is given.

Example:

```sh
curl http://localhost:7711/api/deposits?status=waiting_send
```

Response:

```json
{
    "deposits": [
        {
            "seq": 1,
            "updated_at": 1522494557,
            "status": "waiting_send",
            "coin_type": "BTC",
            "sky_address": "2Wbi4wvxC4fkTYMsS2f6HaFfW4pafDjXcQW",
            "buy_method": "direct",
            "deposit_address": "1PZ63K3G4gZP6A6E2TTbBwxT5bFQGL2TLB",
            "deposit_id": "edb29a9b561a8d6a6118eb1f724c87f853bf471d7e4f0e9ccb9e1d340235687b:11",
            "txid": "",
            "conversion_rate": "500",
            "deposit_value": 201234,
            "sky_sent": 0,
            "passthrough": {
                "exchange_name": "",
                "sky_bought": 0,
                "deposit_value_spent": 0,
                "requested_amount": "",
                "order": {
                    "customer_id": "",
                    "order_id": "",
                    "completed_amount": "",
                    "price": "",
                    "status": "",
                    "final": false,
                    "original": ""
                }
            },
            "error": "",
            "deposit": {
                "coin_type": "BTC",
                "address": "1PZ63K3G4gZP6A6E2TTbBwxT5bFQGL2TLB",
                "value": 201234,
                "height": 494713,
                "tx": "edb29a9b561a8d6a6118eb1f724c87f853bf471d7e4f0e9ccb9e1d340235687b",
                "n": 11,
                "processed": false
            }
        }
    ]
}
```

### Deposit Errors

```sh
Method: GET
URI: /api/deposits/errored
```

Returns all deposits that failed with a permanent error.

Example:

```sh
curl http://localhost:7711/api/deposits/errored
```

Response:

```json
{
    "deposits": [
        {
            "seq": 1,
            "updated_at": 1522494557,
            "status": "done",
            "coin_type": "BTC",
            "sky_address": "2Wbi4wvxC4fkTYMsS2f6HaFfW4pafDjXcQW",
            "buy_method": "direct",
            "deposit_address": "1PZ63K3G4gZP6A6E2TTbBwxT5bFQGL2TLB",
            "deposit_id": "edb29a9b561a8d6a6118eb1f724c87f853bf471d7e4f0e9ccb9e1d340235687b:11",
            "txid": "",
            "conversion_rate": "500",
            "deposit_value": 1,
            "sky_sent": 0,
            "passthrough": {
                "exchange_name": "",
                "sky_bought": 0,
                "deposit_value_spent": 0,
                "requested_amount": "",
                "order": {
                    "customer_id": "",
                    "order_id": "",
                    "completed_amount": "",
                    "price": "",
                    "status": "",
                    "final": false,
                    "original": ""
                }
            },
            "error": "Skycoin send amount is 0",
            "deposit": {
                "coin_type": "BTC",
                "address": "1PZ63K3G4gZP6A6E2TTbBwxT5bFQGL2TLB",
                "value": 201234,
                "height": 494713,
                "tx": "edb29a9b561a8d6a6118eb1f724c87f853bf471d7e4f0e9ccb9e1d340235687b",
                "n": 11,
                "processed": false
            }
        }
    ]
}
```

### Accounting

```sh
Method: GET
URI: /api/accounting
```

Returns the amounts received and sent.

Example:

```sh
curl http://localhost:7711/api/accounting
```

Response:

```json
{
    "sent": "102.943000",
    "received": {
        "BTC": "1.53420000",
        "ETH": "0.000000000000000000",
        "SKY": "0.000000"
    }
}
```

### Backup
```sh
Method: GET
URI: /api/backup
```

Starts a backup download

Example:
```sh
curl -o teller-$(date +%s).db http://localhost:7711/api/backup
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
File: addrs/store.go

Maps: `btcaddr -> ""`
Note: Marks a btc address as used
```

```
Bucket: used_eth_address
File: addrs/store.go

Maps: `ethaddr -> ""`
Note: Marks a eth address as used
```

```
Bucket: exchange_meta
File: exchange/store.go

Note: unused
```

```
Bucket: deposit_info
File: exchange/store.go

Maps: btcTx[%tx:%n]/ethTx[%tx:%n] -> exchange.DepositInfo
Note: Maps a btc/eth txid:seq to exchange.DepositInfo struct
```

```
Bucket: bind_address_BTC
File: exchange/store.go

Maps: btcaddr -> skyaddr
Note: Maps a btc addr to a sky addr
```

```
Bucket: bind_address_ETH
File: exchange/store.go

Maps: ethaddr -> skyaddr
Note: Maps a eth addr to a sky addr
```

```
Bucket: sky_deposit_seqs_index
File: exchange/store.go

Maps: skyaddr -> [btcaddrs/ethaddrs]
Note: Maps a sky addr to multiple btc/eth addrs
```

```
Bucket: btc_txs
File: exchange/store.go

Maps: btcaddr -> [txs]
Note: Maps a btcaddr to multiple btc txns
```

```
Bucket: scan_meta_btc
File: scanner/store.go

Maps: "deposit_addresses" -> [btcaddrs]
Note: Saves list of btc addresss being scanned
```

```
Bucket: scan_meta_eth
File: scanner/store.go

Maps: "deposit_addresses" -> [ethaddrs]
Note: Saves list of eth addresss being scanned

Maps: "dv_index_list" -> [ethTx[%tx:%n]][json]
Note: Saves list of eth txid:seq (as JSON)
```

```
Bucket: deposit_value
File: scanner/store.go

Maps: btcTx/ethTx[%tx:%n] -> scanner.Deposit
Note: Maps a btc/eth txid:seq to scanner.Deposit struct
```

## Frontend development

See [frontend development README](./web/README.md)

## Integration testing

See [integration testing checklist](./integration-testing.md)

## Monitoring logs

To monitor for problems, grep for `ERROR` lines.

For the most critical problems, grep for `notice=WATCH`.
The logging library [logrus](https://github.com/sirupsen/logrus) does not allow
a `CRITICAL` log level; this is our substitute.

## Logrotate integration

Set the `pidfile` in the config and use this pid to have logrotated send a SIGHUP to reopen a log file after being rotated.
See https://github.com/flashmob/go-guerrilla/wiki/Automatic-log-file-management-with-logrotate
for an example logrotate configuration.

## Passthrough notes

Passthrough is still in beta. The service logs must be monitored for errors.

One particular problem is that if an order placed on C2CX enters a failed state,
the system cannot recover automatically.  The operator must resolve the situation
manually.  This is because each order uses a deposit's unique `DepositID` as
the order's unique `CustomerID`, so an order cannot be resubmitted for a given
deposit, since the `CustomerID` cannot be reused.  It is also unclear if an order
can be partially completed, then enter a failed state. In this case, some of the
deposit's value may have been spent on the exchange and we would not want to
resubmit an order for the entire original deposit value.
