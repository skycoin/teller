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
        "min_scan_block": 2,
        "mid_scan_block": 2,
        "max_scan_block": 2,
        "txs": [
            {
                "tx_hash": "0e3e2357e806b6cdb1f70b54c3a3a17b6714ee1f0e68bebb44a74b1efd512098:0",
                "block_hash": "00000000839a8e6886ab5951d76f411475428afc90947ee320161bbf18eb6048",
                "parent_hash": "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f",
                "block_height": 1,
                "satoshi_amount": 5000000000,
                "bitcoin_amount": "50.00000000"
            }
        ]
    },
    {
        "address": "114zv1fDSaQTA2xyCtGR5pxN19tX5m92yw",
        "min_scan_block": 0,
        "mid_scan_block": 125688,
        "max_scan_block": 125688,
        "txs": [
            {
                "tx_hash": "2e2f0f58138a8cdb5bd681f2eeeac8a25b162deee205058a3710a1bbf644dba8:0",
                "block_hash": "00000000000007e717d8dcbc9738a9e98c3d1fe425441ccd75c1de359b480d7f",
                "parent_hash": "0000000000000341ab18e798a8c0e4d8039693eb48b8a35f091a8b03860e0324",
                "block_height": 125688,
                "satoshi_amount": 66000000,
                "bitcoin_amount": "0.66000000"
            }
        ]
    },
    {
        "address": "1Bj2A5qERFaLkVrvB9ptuu18UB1NqpeWMx",
        "min_scan_block": 0,
        "mid_scan_block": 266588,
        "max_scan_block": 266588,
        "txs": [
            {
                "tx_hash": "4e1c8df32926a9704fbc4cdb21bc117c994a6821ff1d4f359477e68154480e0c:0",
                "block_hash": "000000000000000aa39aeae6a76d173bed773dc0b85a53ecbb39aefb2ada150b",
                "parent_hash": "0000000000000008611db6252e1677c7d974ea9e3d91b4e1db49c2bf02f11da8",
                "block_height": 266588,
                "satoshi_amount": 1531647,
                "bitcoin_amount": "0.01531647"
            }
        ]
    }
]
```