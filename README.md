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
    - [Generate BTC addresses](#generate-btc-addresses)
    - [Generate ETH addresses](#generate-eth-addresses)
    - [Setup skycoin hot wallet](#setup-skycoin-hot-wallet)
    - [Run teller](#run-teller)
    - [Setup skycoin node](#setup-skycoin-node)
    - [Setup btcd](#setup-btcd)
        - [Configure btcd](#configure-btcd)
        - [Obtain btcd RPC certificate](#obtain-btcd-rpc-certificate)
    - [Using a reverse proxy to expose teller](#using-a-reverse-proxy-to-expose-teller)
    - [Setup geth](#setup-geth)
        - [Configure geth](#configure-geth)
    - [you can using a reverse proxy to expose geth rpc port such as Using a reverse proxy to expose teller](#you-can-using-a-reverse-proxy-to-expose-geth-rpc-port-such-as-using-a-reverse-proxy-to-expose-teller)
- [API](#api)
    - [Bind](#bind)
    - [Status](#status)
    - [Config](#config)
    - [Exchange Status](#exchange-status)
    - [Dummy](#dummy)
        - [Scanner](#scanner)
            - [Deposit](#deposit)
        - [Sender](#sender)
            - [Broadcasts](#broadcasts)
            - [Confirm](#confirm)
- [Code linting](#code-linting)
- [Run tests](#run-tests)
- [Database structure](#database-structure)
- [Frontend development](#frontend-development)
- [Integration testing](#integration-testing)

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
* `btc_addresses` [string]: Filepath of the btc_addresses.json file. See [generate BTC addresses](#generate-btc-addresses).
* `eth_addresses` [string]: Filepath of the eth_addresses.json file. See [generate ETH addresses](#generate-eth-addresses).
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
* `sky_exchanger.sky_btc_exchange_rate` [string]: How much SKY to send per BTC. This can be written as an integer, float, or a rational fraction.
* `sky_exchanger.max_decimals` [int]: Number of decimal places to truncate SKY to.
* `eth_rpc.server` [string]: Host address of the geth node.
* `eth_rpc.port` [string]: Host port of the geth node.
* `eth_scanner.scan_period` [duration]: How often to scan for ethereum blocks.
* `eth_scanner.initial_scan_height` [int]: Begin scanning from this ETH blockchain height.
* `eth_scanner.confirmations_required` [int]: Number of confirmations required before sending skycoins for a ETH deposit.
* `sky_exchanger.sky_eth_exchange_rate` [string]: How much SKY to send per ETH. This can be written as an integer, float, or a rational fraction.
* `sky_exchanger.wallet` [string]: Filepath of the skycoin hot wallet. See [setup skycoin hot wallet](#setup-skycoin-hot-wallet).
* `sky_exchanger.tx_confirmation_check_wait` [duration]: How often to check for a sent skycoin transaction's confirmation.
* `sky_exchanger.send_enabled` [bool]: Disable this to prevent sending of coins (all other processing functions normally, e.g.. deposits are received)
* `sky_exchanger.buy_method` [string]: Options are "direct" or "passthrough". "direct" will send directly from the wallet. "passthrough" will purchase from an exchange before sending from the wallet.
* `web.behind_proxy` [bool]: Set true if running behind a proxy.
* `web.static_dir` [string]: Location of static web assets.
* `web.throttle_max` [int]: Maximum number of API requests allowed per `web.throttle_duration`.
* `web.throttle_duration` [int]: Duration of throttling, pairs with `web.throttle_max`.
* `web.http_addr` [string]: Host address to expose the HTTP listener on.
* `web.https_addr` [string] Host address to expose the HTTPS listener on.
* `web.auto_tls_host` [string]: Hostname/domain to install an automatic HTTPS certificate for, using Let's Encrypt.
* `web.tls_cert` [string]: Filepath to TLS certificate. Cannot be used with `web.auto_tls_host`.
* `web.tls_key` [string]: Filepath to TLS key. Cannot be used with `web.auto_tls_host`.
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

### Generate BTC addresses

Use `tool` to pregenerate a list of bitcoin addresses in a JSON format parseable by teller:

```sh
go run cmd/tool/tool.go -json newbtcaddress $seed $num > addresses.json
```

Name the `addresses.json` file whatever you want.  Use this file as the
value of `btc_addresses` in the config file.

### Generate ETH addresses

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
then put those address into `eth_addresses.json` which format like `btc_addresses.json`

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

```
Building geth requires both a Go (version 1.7 or later) and a C compiler.
You can install them using your favourite package manager.
Once the dependencies are installed, run
cd go-ethereum
make geth
copy geth to you system PATH, such as: cp build/bin/geth /usr/bin

```

#### Configure geth

specified the following values:

* `--datadir` - set this to directory that store ethereum block chain data.
* `--rpc` - Enable the HTTP-RPC server
* --rpcaddr` - HTTP-RPC server listening interface (default: "localhost") Set this as the value of `eth_rpc.server` in the teller conf.
* `--rpcport`- HTTP-RPC server listening port (default: 8545) Set this as the value of `eth_rpc.port` in the teller conf.

```
Please note, offering an API over the HTTP (rpc) or WebSocket (ws) interfaces will give
everyone access to the APIs who can access this interface (DApps, browser tabs, etc). Be
careful which APIs you enable. By default Geth enables all APIs over the IPC (ipc) interface
and only the db, eth, net and web3 APIs over the HTTP and WebSocket interfaces.

```

Ethereum Api See https://github.com/ethereum/go-ethereum/wiki/Management-APIs

### you can using a reverse proxy to expose geth rpc port such as [Using a reverse proxy to expose teller](#using-a-reverse-proxy-to-expose-teller)
Now, run `geth`:

```sh
geth --datadir=xxx

as a daemon
nohup geth --datadir=xxxx > geth.log 2>&1 &
```
## API

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

Example:

```sh
curl http://localhost:7071/api/config
```

Response:

```json
{
    "enabled": true,
    "btc_confirmations_required": 1,
    "eth_confirmations_required": 5,
    "max_bound_addrs": 5,
    "max_decimals": 0,
    "sky_btc_exchange_rate": "123.000000"
    "sky_eth_exchange_rate": "30.000000"
}
```

### Exchange Status

```sh
Method: GET
Content-Type: application/json
URI: /api/exchange-status
```

Return the exchanger's status.
The exchanger's status is the last error seen while trying to send SKY, or nil if no last error was seen.
Use this to detect if the OTC is temporarily offline or sold out.

The balance of the OTC wallet is included in the response.  The wallet may still be considered "sold out" even
if there is a balance remaining. The wallet is considered "sold out" when there are not enough coins in the wallet
to satisfy the current purchase, so it is unlikely for the wallet to ever reach exactly 0.  For example, if there
are 100 coins in the wallet and someone attempts to purchase 200 coins, it will be considered "sold out".
In this case, the "error" field will be set to some message string, and the balance will say "100.000000".

Example:

```sh
curl http://localhost:7071/api/exchange-status
```

Response:

```json
{
    "error": "",
    "balance": {
        "coins": "100.000000",
        "hours": "100",
    }
}
```

```json
{
    "error": "no unspents to spend",
    "balance": {
        "coins": "0.000000",
        "hours": "0",
    }
}
```


Possible statuses are:
TODO

### Dummy

A dummy scanner and sender API is available over `dummy.http_addr` if
`dummy.scanner` or `dummy.sender` are enabled.

#### Scanner

##### Deposit

```sh
Method: GET, POST
URI: /dummy/scanner/deposit
```

Adds a deposit to the scanner.

Example:

```sh
curl http://localhost:4121/dummy/scanner/deposit?addr=1PZ63K3G4gZP6A6E2TTbBwxT5bFQGL2TLB&value=100000000&height=494713&tx=edb29a9b561a8d6a6118eb1f724c87f853bf471d7e4f0e9ccb9e1d340235687b&n=0
```

#### Sender

##### Broadcasts

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

##### Confirm

```sh
Method: GET, POST
URI: /dummy/sender/confirm
```

Confirms a broadcasted transaction.

Example:

```sh
curl http://localhost:4121/dummy/sender/confirm?txid=4fc9743b04c2e3f5e467cde38c0872e3e3ad9ec05d59081ad1a8bd88045635de
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
