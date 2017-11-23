# Integration testing checklist

## If the BTC deposit is an exact multiple of the BTC/SKY rate, the SKY sent is exact

* Start teller with a btcd node and skycoin node running
* Bind an address
* Send BTC equal to an exact multiple of the SKY/BTC rate.

For example, if the rate is 0.0001 BTC/SKY, send 0.002 BTC and receive 2 SKY.

The exact SKY amount should be sent.

## If the BTC deposit is not an exact multiple of the BTC/SKY rate, the SKY sent is rounded down

* Start teller with a btcd node and skycoin node running
* Bind an address
* Send BTC in an inexact multiple of the SKY/BTC rate.

For example, if the rate is 0.001 BTC/SKY, send 0.0025 BTC and receive 2 SKY.

The SKY amount should be sent rounded down.

## If the BTC deposit is less than the price of the minimum unit of SKY, no SKY is sent

* Start teller with a btcd node and skycoin node running
* Bind an address
* Send less BTC than the price of the minimum unit of SKY.

For example, if the rate is 0.001 BTC/SKY and the minimum unit is 1 SKY,
send 0.0009 BTC and receive 0 SKY.

No SKY should be sent.

## Multiple BTC deposits to the same address are processed

* Start teller with a btcd node and skycoin node running
* Bind an address
* Send BTC to the address twice, in two separate transactions
* Wait for this deposit to process
* Confirm that both deposits were in the same block, in case the bitcoin network put them in different blocks
* Send BTC to the address again
* Wait for this deposit to process

All three deposits should process.

## Multiple BTC deposits in one block are processed

* Start teller with a btcd node and skycoin node running
* Bind 2 addresses
* Send BTC to both address, quickly enough so that they are in the same block
* Wait for these deposits to process
* Confirm that both deposits were in the same block, in case the bitcoin network put them in different blocks

Both deposits should process.

## If the btcd node is not available during teller startup, teller does not run

* Start teller without running a btcd node, but with a skycoin node

Teller should fail to start.

## If the btcd node becomes temporarily unavailable, teller is unaffected

* Start teller with a btcd node and skycoin node running
* Bind an address
* Stop the btcd node
* Make a BTC deposit
* Wait for the deposit to confirm at least 1 block
* Restart the btcd node

Teller should start up and begin scanning deposits.
Then it will fail to scan more, but not exit.
When btcd is restarted, scanning will resume, and the deposit will process.

## If the skycoin node is not available during teller startup, teller does not run

* Start teller without running a skycoin node, but with a btcd node

Teller should fail to start

## If the skycoin node becomes temporarily unavailable, teller is unaffected

* Start teller with a btcd node and skycoin node running
* Bind an address
* Stop the skycoin node
* Make a BTC deposit
* Wait for the deposit to be scanned
* Wait for an error message about SKY failed to send
* Restart the skycoin node

Teller should start up and begin scanning deposits.
Then it will scan the BTC deposit, and send it to the exchanger.
The exchanger will save the deposit and try to send SKY.
Sending SKY will fail while the skycoin node is offline.
When the skycoin node is restarted, the send will occur.

## If teller is shutdown before it sends SKY, it will send the SKY when it restarts, and at the SKY/BTC rate that was set when the deposit was processed

* Start teller with a btcd node and skycoin node running
* Bind an address
* Stop the skycoin node
* Make a BTC deposit
* Wait for the deposit to be scanned
* Wait for an error message about SKY failed to send
* Restart teller with a different SKY/BTC rate

Teller will resume sending SKY for scanned deposits when it restarts.
When a deposit is processed, the current SKY/BTC rate is saved with the deposit.
Teller will resume sending SKY based on the SKY/BTC rate at the time, so if the
configurable rate has been changed, that deposit receives the expected rate.

## When teller is restarted, rescanned deposits do not process again

* Start teller with a btcd node and skycoin node running
* Bind an address
* Make a BTC deposit
* Wait for the deposit to be processed
* Restart teller
* Look for a message about deposit already processed

Teller scans blocks from a fixed BTC blockchain height.  When it restarts,
it will rescan blocks that it has already scanned.  When it detects a deposit
that has already been processed, it will not process this deposit again.
