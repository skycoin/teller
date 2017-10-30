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
 
