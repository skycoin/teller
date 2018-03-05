# Integration testing checklist

## If the BTC deposit is an exact multiple of the BTC/MDL rate, the MDL sent is exact

* Start teller with a btcd node and mdl node running
* Bind an address
* Send BTC equal to an exact multiple of the MDL/BTC rate.
* Check that the deposit is "done" using the status check in the web client
* Check that the MDL address receives the expected amount

For example, if the rate is 0.0001 BTC/MDL, send 0.002 BTC and receive 2 MDL.

The exact MDL amount should be sent.

## If the BTC deposit is not an exact multiple of the BTC/MDL rate, the MDL sent is rounded down

* Start teller with a btcd node and mdl node running
* Bind an address
* Send BTC in an inexact multiple of the MDL/BTC rate.

For example, if the rate is 0.001 BTC/MDL, send 0.0025 BTC and receive 2 MDL.

The MDL amount should be sent rounded down.

## If the BTC deposit is less than the price of the minimum unit of MDL, no MDL is sent

* Start teller with a btcd node and mdl node running
* Bind an address
* Send less BTC than the price of the minimum unit of MDL.
* Check that the deposit is "done" using the status check in the web client
* Check that the MDL address did not receive anything

For example, if the rate is 0.001 BTC/MDL and the minimum unit is 1 MDL,
send 0.0009 BTC and receive 0 MDL.

No MDL should be sent.

## Multiple BTC deposits to the same address are processed

* Start teller with a btcd node and mdl node running
* Bind an address
* Send BTC to the address twice, in two separate transactions
* Wait for this deposit to process
* Confirm that both deposits were in the same block, in case the bitcoin network put them in different blocks
* Send BTC to the address again
* Wait for this deposit to process
* Check that the deposits are "done" using the status check in the web client
* Check that the MDL address receives the expected amount

All three deposits should process.

## Multiple BTC deposits in one block are processed

* Start teller with a btcd node and mdl node running
* Bind an address to two different MDL addresses each
* Send BTC to both address, quickly enough so that they are in the same block
* Wait for these deposits to process
* Confirm that both deposits were in the same block, in case the bitcoin network put them in different blocks
* Check that the deposits are "done" using the status check in the web client
* Check that the MDL addresses receives the expected amount

Both deposits should process.

## If the btcd node is not available during teller startup, teller does not run

* Start teller without running a btcd node, but with a mdl node

Teller should fail to start.

## If the btcd node becomes temporarily unavailable, teller is unaffected

* Start teller with a btcd node and mdl node running
* Bind an address
* Stop the btcd node
* Make a BTC deposit
* Wait for the deposit to confirm at least 1 block
* Restart the btcd node
* Check that the deposit is "done" using the status check in the web client
* Check that the MDL address receives the expected amount

Teller should start up and begin scanning deposits.
Then it will fail to scan more, but not exit.
When btcd is restarted, scanning will resume, and the deposit will process.

## If the mdl node is not available during teller startup, teller does not run

* Start teller without running a mdl node, but with a btcd node

Teller should fail to start

## If the mdl node becomes temporarily unavailable, teller is unaffected

* Start teller with a btcd node and mdl node running
* Bind an address
* Stop the mdl node
* Make a BTC deposit
* Wait for the deposit to be scanned
* Wait for an error message about MDL failed to send
* Restart the mdl node
* Check that the deposit is "done" using the status check in the web client
* Check that the MDL address receives the expected amount

Teller should start up and begin scanning deposits.
Then it will scan the BTC deposit, and send it to the exchanger.
The exchanger will save the deposit and try to send MDL.
Sending MDL will fail while the mdl node is offline.
When the mdl node is restarted, the send will occur.

## If teller is shutdown before it sends MDL, it will send the MDL when it restarts, and at the MDL/BTC rate that was set when the deposit was processed

* Start teller with a btcd node and mdl node running
* Bind an address
* Stop the mdl node
* Make a BTC deposit
* Wait for the deposit to be scanned
* Wait for an error message about MDL failed to send
* Restart teller with a different MDL/BTC rate
* Check that the deposit is "done" using the status check in the web client
* Check that the MDL address receives the expected amount at the original rate

Teller will resume sending MDL for scanned deposits when it restarts.
When a deposit is processed, the current MDL/BTC rate is saved with the deposit.
Teller will resume sending MDL based on the MDL/BTC rate at the time, so if the
configurable rate has been changed, that deposit receives the expected rate.

## When teller is restarted, rescanned deposits do not process again

* Start teller with a btcd node and mdl node running
* Bind an address
* Make a BTC deposit
* Wait for the deposit to be processed
* Check that the deposit is "done" using the status check in the web client
* Check that the MDL address receives the expected amount
* Restart teller
* Look for a log message about deposit already processed
* Check that the deposit is "done" using the status check in the web client
* Check that the MDL address did not receive more coins

Teller scans blocks from a fixed BTC blockchain height.  When it restarts,
it will rescan blocks that it has already scanned.  When it detects a deposit
that has already been processed, it will not process this deposit again.

## If the mdl wallet runs out of funds, pending transactions will resume when refilled

### Case 1: Teller is not stopped between refills

* Start teller with a btcd node and mdl node running
# Use an empty mdl wallet
* Bind an address
* Make a BTC deposit
# Look for an error when creating the transaction
* Add sufficient coins to the mdl wallet
* Check that the deposit is "done" using the status check in the web client
* Check that the MDL address receives the expected amount

### Case 2: Teller is stopped between refills

* Start teller with a btcd node and mdl node running
# Use an empty mdl wallet
* Bind an address
* Make a BTC deposit
# Look for an error when creating the transaction
* Stop teller
* Add sufficient coins to the mdl wallet
# Start teller
* Check that the deposit is "done" using the status check in the web client
* Check that the MDL address receives the expected amount

Teller will repeatedly try to create a transaction and send it until it succeeds.
It will resume doing this between restarts.

## If the BTC address pool runs out of addresses, a clear error is provided to the client and an error is logged

* Start teller with a btcd node and mdl node running
* Use an empty btc_addresses.json file
* Bind an address
* Observe the error message in the client and in the logs

## If the user binds too many addresses, a clear error is provided to the client and an error is logged

* Start teller with a btcd node and mdl node running
* Use an empty btc_addresses.json file
* Bind an address
* Observe the error message in the client and in the logs
