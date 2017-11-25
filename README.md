![teller logo](https://user-images.githubusercontent.com/26845312/32426949-3682cd18-c283-11e7-9067-55084310064d.png)

# Teller

[![Build Status](https://travis-ci.org/skycoin/teller.svg?branch=master)](https://travis-ci.org/skycoin/teller)

<!-- MarkdownTOC autolink="true" bracket="round" depth="5" -->

- [Releases & Branches](#releases--branches)
- [Setup project](#setup-project)
    - [Prerequisites](#prerequisites)
    - [Configure teller](#configure-teller)
    - [Running teller without btcd or skyd](#running-teller-without-btcd-or-skyd)
    - [Generate BTC addresses](#generate-btc-addresses)
    - [Setup skycoin hot wallet](#setup-skycoin-hot-wallet)
    - [Run teller](#run-teller)
    - [Setup skycoin node](#setup-skycoin-node)
    - [Setup btcd](#setup-btcd)
        - [Configure btcd](#configure-btcd)
        - [Obtain btcd RPC certificate](#obtain-btcd-rpc-certificate)
    - [Using a reverse proxy to expose teller](#using-a-reverse-proxy-to-expose-teller)
- [API](#api)
    - [Bind](#bind)
    - [Status](#status)
    - [Config](#config)
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

### Configure teller

The config file can be specified with `-c` or `--config` from the command line.

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
* `teller.max_bound_btc_addrs` [int]: Maximum number of BTC addresses allowed to bind per skycoin address.
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
* `sky_exchanger.wallet` [string]: Filepath of the skycoin hot wallet. See [setup skycoin hot wallet](#setup-skycoin-hot-wallet).
* `sky_exchanger.tx_confirmation_check_wait` [duration]: How often to check for a sent skycoin transaction's confirmation.
* `web.behind_proxy` [bool]: Set true if running behind a proxy.
* `web.api_enabled` [bool]: Set true to enable the teller API. Disable it if you want to expose the frontend homepage, but not allow people to access the teller service.
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

### Running teller without btcd or skyd

Teller can be run in "dummy mode". It will ignore btcd and skycoind.
It will still provide addresses via `/api/bind` and report status with `/api/status`.
but it will not process any real deposits or send real skycoins.

See the [dummy API](#dummy) for controlling the fake deposits and sends.

### Generate BTC addresses

Use `tool` to pregenerate a list of bitcoin addresses in a JSON format parseable by teller:

```sh
go run cmd/tool/tool.go -json newbtcaddress $seed $num > addresses.json
```

Name the `addresses.json` file whatever you want.  Use this file as the
value of `btc_addresses` in the config file.

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

Binds a skycoin address to a BTC address. A skycoin address can be bound to
multiple BTC addresses. The default maximum number of bound addresses is 5.

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

## Frontend development

See [frontend development README](./web/README.md)

## Integration testing

See [integration testing checklist](./integration-testing.md)
