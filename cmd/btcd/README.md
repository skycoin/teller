# btcd simulator

The btcd simulator imitates a btcd node.  A teller instance can connect to it
the same way as it would connect to a real btcd node.
It responds with simulated data to the RPC calls that teller makes.

## Running the btcd simulator

```sh
go run cmd/btcd/btcd.go
```

The first time it is run, it generates self-signed certificates to use.
You can reuse these with the `-cert` and `-key` parameters.

Configure teller to use the cert file for its btcd RPC.

Username and password are ignored by the simulated btcd.

The teller config's `btc_scanner.initial_scan_height` must be set to `492478`.
The btcd simulator begins returning blocks from this height.

## API

The default API address is `http://localhost:8834"`

### Generate a deposit

```
URI: /api/nextdeposit
Method: POST
Content-Type: application/json
Request Body: [{
    "Address": "btc address 1",
    "Value": 123,
    "N": 1,
}, {
    "Address": "btc address 2",
    "Value": 456,
    "N": 0,
}]
```

The request body is an array of objects specifying a BTC address, an integer amount measured in satoshis,
and the index of the output inside a transaction.

Each request generates a new block. Multiple objects included in a single request will appear in the same block.
The index value can be any number, but must be unique amongst all objects in the array.

Example:

```sh
curl -d '[{"Address":"1PZ63K3G4gZP6A6E2TTbBwxT5bFQGL2TLB","Value":100000000,"N":1}]' -H "Content-Type: application/json" -X POST http://127.0.0.1:8834/api/nextdeposit
```
