# Scanner for bitcoin wallet

## Setup project

### Prerequisites

* Have go1.8+ installed
* Have `GOPATH` env set
* Have btcd started
* Add addresses to wallet.json in right format


### Installing btcd

- Run the following commands to obtain btcd, all dependencies, and install it:

```bash
$ go get -u github.com/Masterminds/glide
$ git clone https://github.com/btcsuite/btcd $GOPATH/src/github.com/btcsuite/btcd
$ cd $GOPATH/src/github.com/btcsuite/btcd
$ glide install
$ go install . ./cmd/...
```
- btcd (and utilities) will now be installed in $GOPATH/bin. Go where and

```bash
$ ./btcd
```


### Start scanning and other options

This utility have several flags:

```
-n first blockID for scan
-m last blockID for scan
-wallet path to wallet.json
-user btcd username
-pass btcd password
-add get addresses and put in watching list
-add_file get addresses from json file
-upd_min bool expression for update min block
-upd_max bool expression for update max block
-rand1 bool expression for update min(max-min)
-rand2 bool expression for update min(mid-min)
-randomize bool expression for random update(min/max/rand1/rand2)
```
### Example usage

```
go run scan.go -n=1 -m=5
go run scan.go -add=17abzUBJr7cnqfnxnmznn8W38s9f9EoXiq,1DMGtVnRrgZaji7C9noZS3a1QtoaAN2uRG
go run scan.go -upd_min=true
```
If you want to add addresses from json file shoud be in format:

```
{
  "btc_addresses": [
    "1PZ63K3G4gZP6A6E2TTbBwxT5bFQGL2TLB",
    "1HLoD9E4SDFFPDiYfNYnkBLQ85Y51J3Zb1",
    "1Mv16pwUZYUrMWLTe2DDZzXHGAyHdKA5oz",
    "1NvBwUKqUuH3HbPjHq417XhQ551RHhogso",
    "1Kar4VK9HLkcQ99iWbs4LuCGEyDdTab5PC"
  ]
}
```
For loading from file use command:

```bash
$ go run scan.go -add_file=btc_addresses.json
```


Also you can combine commands

```
go run scan.go -add=1CYG7y3fukVLdobqgUtbknwWKUZ5p1HVmV -n=10 -m=16
```
 
### Formats

empty wallet.json shout be in right format:

```
[
    {
        "address": "12c6DSiU4Rq3P4ZxziKxzrL5LmMBrzjrJX",
        "min_scan_block": 0,
        "mid_scan_block": 0,
        "max_scan_block": 0,
        "txs": []
    }
]
```

After you will add some addresses and make scans, it looks something like:

```
[
    {
        "address": "12c6DSiU4Rq3P4ZxziKxzrL5LmMBrzjrJX",
        "min_scan_block": 29,
        "mid_scan_block": 29,
        "max_scan_block": 29,
        "txs": [
            {
                "tx_hash": "0e3e2357e806b6cdb1f70b54c3a3a17b6714ee1f0e68bebb44a74b1efd512098:0",
                "btc_address": "12c6DSiU4Rq3P4ZxziKxzrL5LmMBrzjrJX",
                "block_hash": "00000000839a8e6886ab5951d76f411475428afc90947ee320161bbf18eb6048",
                "parent_hash": "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f",
                "block_height": 1,
                "satoshi_amount": 5000000000,
                "bitcoin_amount": "50"
            }
        ]
    },
    {
        "address": "1L5sCUfn36yfRByzY4qJLNuJJfRaJg3Vwj",
        "min_scan_block": 29,
        "mid_scan_block": 120120,
        "max_scan_block": 120130,
        "txs": [
            {
                "tx_hash": "ca29ebc25bc288d6f18dc730e553d6425d6524d56bb2d94baf6acbac0c619d59:0",
                "btc_address": "1L5sCUfn36yfRByzY4qJLNuJJfRaJg3Vwj",
                "block_hash": "000000000000a0ec8ed82994c0682c4e4b8e99f5dc9595180eef3da5e358d0da",
                "parent_hash": "00000000000022ee82271955109bcb6f11d03c58990530429a4ba3ce7edbdfba",
                "block_height": 120120,
                "satoshi_amount": 2370000000,
                "bitcoin_amount": "23.7"
            }
        ]
    }
]
```