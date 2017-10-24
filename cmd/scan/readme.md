# Scanner for bitcoin wallet

## Setup project

### Prerequisites

* Have go1.8+ installed
* Have `GOPATH` env set
* Have btcd started
* Add addresses to wallet.json in right format



### Correct format for adding new addresses
```bash
[
    {
        "address": "12c6DSiU4Rq3P4ZxziKxzrL5LmMBrzjrJX",
        "min_scan_block": 0,
        "mid_scan_block": 0,
        "max_scan_block": 0,
        "txs": [
            {
                "hash": "",
                "id": -1,
                "block_hash": "",
                "parent_hash": "",
                "block_height": 0
            }
        ]
    }
]
```


### Start scanning

This utility have several flags:

```
-n first blockID for scan
-m last blockID for scan
-wallet path to wallet.json
```
### Example usage

```
go run scan.go -n=1 -m=5
```

 
