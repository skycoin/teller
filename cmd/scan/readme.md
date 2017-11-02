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
```
### Example usage

```
go run scan.go -n=1 -m=5
go run scan.go -add=17abzUBJr7cnqfnxnmznn8W38s9f9EoXiq,1DMGtVnRrgZaji7C9noZS3a1QtoaAN2uRG
```
If you want add addresses from json file shoud be in format:

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
        "min_scan_block": 5,
        "mid_scan_block": 288888,
        "max_scan_block": 288890,
        "txs": [
            {
                "tx_hash": "0e3e2357e806b6cdb1f70b54c3a3a17b6714ee1f0e68bebb44a74b1efd512098:0",
                "block_hash": "00000000839a8e6886ab5951d76f411475428afc90947ee320161bbf18eb6048",
                "parent_hash": "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f",
                "block_height": 1
            }
        ]
    },
    {
        "address": "1EXoDusjGwvnjZUyKkxZ4UHEf77z6A5S4P",
        "min_scan_block": 5,
        "mid_scan_block": 288888,
        "max_scan_block": 288890,
        "txs": [
            {
                "tx_hash": "500f2be2ebd5193578de1e521fbd7a3d9849a8983a70a8bef89e3d0a89a9a18b:1",
                "block_hash": "0000000000000000542170ae05be423a9d4a5ad6b8a933d34dc12856f05391cd",
                "parent_hash": "0000000000000000a456b9cd160ccfaf4d6b7341dc9aae04f98e5120fa5a73a3",
                "block_height": 288889
            },
            {
                "tx_hash": "beb620dfe9900f0961739c6eb60581d2882f6cbc91039ee6f1b5690fcb3bcded:1",
                "block_hash": "0000000000000000542170ae05be423a9d4a5ad6b8a933d34dc12856f05391cd",
                "parent_hash": "0000000000000000a456b9cd160ccfaf4d6b7341dc9aae04f98e5120fa5a73a3",
                "block_height": 288889
            }
        ]
    },
    {
        "address": "1FtuEk12fmeGJKJ1BEJ2CYMkHPUEgpkLgi",
        "min_scan_block": 5,
        "mid_scan_block": 455454,
        "max_scan_block": 455455,
        "txs": [
            {
                "tx_hash": "ac6061708c704a665ab005f0fedb9ad5d297c6afa4a42cf49abba4ddc1b88354:1",
                "block_hash": "000000000000000000addd7303c43756a7e66ebe30d5a32a75bc87ad67027390",
                "parent_hash": "000000000000000002655615168e2f0e2c1d7c2798a128f757928efd0954a333",
                "block_height": 455454
            }
        ]
    }
]
```