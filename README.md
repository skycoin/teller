# teller

There're mainly two processes in this project, ico server and ico proxy.

## ICO-SERVER

The ico-server will run in a private environment for security, which can't be accessed 
from public network directly, cause the ico coin's wallet is located here, and contains 
the private keys of all addresses in the wallet.

Once the ico-server started, it will try to connect to the ico-proxy, and establish
an encrypted session, all data transferred in this session are encrypted with ECDH and
chacha20.

### Run ico-server

#### Prerequest:
1. have go1.7+ installed
2. have skycoin-cli(command line interface) installed
3. have local skycoin node started

To install the CLI:

```bash
$ cd $GOPATH/src/github.com/skycoin/teller/cmd/server
$ ./install.sh
```

To install the skycoin node, please follow the instructions [here](https://github.com/skycoin/skycoin/blob/master/README.md).

#### Config

The config.json file is located in the folder of `$GOPATH/src/github.com/skycoin/teller/cmd/server`

```json
{
    "proxy_address": "127.0.0.1:7070",
    "reconnect_time": 5,
    "dial_timeout": 5,
    "ping_timeout": 5,
    "pong_timeout": 10,
    "exchange_rate": [
        {
            "date": "0001-01-01 00:00:00",
            "rate":2500
        },
    ],
    "monitor": {
        "check_period": 20
    },
    "node": {
        "rpc_address": "127.0.0.1:7430",
        "wallet_path": "/skycoin/wallets/skycoin.wlt"
    },
    "deposit_coin": "bitcoin",
    "ico_coin": "skycoin",
}
```

The `proxy_address` is the ico-proxy server address. All time related value are using `seconds`
as unit, example: the `dial_timeout: 5`, means the dial timeout value is 5 seconds.

The `exchange_rate` field represents the exchange rate between deposit coin and ico coin.
For example, if the rate is 2500 which means deposit 1 `bitcoin` can get 2500 `skycoin`.

`check_period` field in `monitor` represents the time interval for checking the deposit coin balance.
We use the api from https://blockexplorer.com, so please don't set it too short.

The value of `rpc_address` is the default rpc address of local skycoin node, you can ignore it if
you didn't change the rpc port.

`wallet_path` must be an absolute path of an real skycoin wallet in your computer.


#### Start server

```bash
$ cd $GOPATH/src/github.com/skycoin/teller/cmd/server
$ go run server.go
```

## ICO-PROXY

The ico-proxy is run as a service in public network environment, which will provid 
http apis for web-server end. When ico-proxy recv http api requet, it will send 
corresponding request to ico-server and send ack from ico-server back to the http
client.

As we described in the prevous section, the session between ico-server and ico-proxy
is encrypted, so it's safe, while the http request from web-server to ico-proxy is
not safe, so we recommend to run the http api service in localhost, and the web-server
should run in the same machine as ico-proxy, in this way, we can both avoid invalid
http request from public network, and provid api service for web-server.

### Run ico-proxy

```bash
$ cd $GOPATH/src/github.com/skycoin/teller/cmd/proxy
$ go run proxy.go
```

The proxy will open an public tcp port on `0.0.0.0:7070`, which is used to communicate
with the ico-server, and also have an http service on `localhost:7071`.

### web service apis

#### Add monitor

```
URI: /monitor
Method: POST
Content-Type: application/json
Body:
{
    "id": 0,
    "deposit_coin": {
        "coin": "BTC",
        "address": "1EVES5roY8zspu9s632PeJB43MRNJ1oiuJ"
    },
    "ico_coin": {
        "coin": "SKY",
        "address": "cBB6zToAjhMkvRrWaz8Tkv6QZxjKCfJWBx"
    }
}

```

#### Get exchange logs

```
URI: /exchange_logs?start=1&end=100
Method: GET

```

Note: the log id is start from 1, so the `start` should begin with 1 as well.

Result example:

```json
{
  "id": 0,
  "result": {
    "success": true,
    "err": ""
  },
  "max_log_id": 3,
  "exchange_logs": [
    {
      "logid": 1,
      "time": "2017-04-29T23:11:44.71962344+08:00",
      "deposit": {
        "Address": "bitcoin",
        "Coin": 10000
      },
      "ico": {
        "Address": "skycoin",
        "Coin": 2
      },
      "tx": {
        "hash": "1a0a659782c46be096c0ea1cec4285d04594bc78a93231665f0ae1b2685df639",
        "coinfirmed": true
      }
    },
    {
      "logid": 2,
      "time": "2017-04-29T23:13:15.104516523+08:00",
      "deposit": {
        "Address": "bitcoin",
        "Coin": 10000
      },
      "ico": {
        "Address": "skycoin",
        "Coin": 2
      },
      "tx": {
        "hash": "541aa8bfcafb7c4b404c0004b714c5efb922393a8c87f647057ad80f6fc0e717",
        "coinfirmed": true
      }
    }
  ]
}
```

The `max_log_id` represents current maximum log id. For bitcoin, the Coin unit is `statoshi`, and
`sky` fro skycoin.
