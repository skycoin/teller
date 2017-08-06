export default {
  distribution: {
    title: 'Skycoin distribution event',
    heading: 'Skycoin distribution event',
    instructions: `
<p>To participate in the distribution event:</p>

<ul>
  <li>Enter your Skycoin address below</li>
  <li>You&apos;ll receive a unique Bitcoin address to purchase SKY</li>
  <li>Send BTC to the address—you&apos;ll receive 1 SKY per 0.002 BTC</li>
</ul>

<p>You can check the status of your order by entering your address and selecting <strong>Check status</strong>.</p>
    `,
    btcAddressFor: 'BTC address for {skyAddress}',
    statusFor: 'Status for {skyAddress}',
    enterAddress: 'Enter Skycoin address',
    getAddress: 'Get address',
    checkStatus: 'Check status',
    errors: {
      noSkyAddress: 'Please enter your SKY address.',
    },
    statuses: {
      done: 'Transaction {id}: Skycoin deposit confirmed (updated at {updated}).',
      waiting_deposit: 'Transaction {id}: waiting for BTC deposit (updated at {updated}).',
      waiting_send: 'Transaction {id}: BTC deposit received; Skycoin deposit is queued (updated at {updated}).',
      waiting_confirm: 'Transaction {id}: Skycoin deposit sent; waiting to confirm transaction (updated at {updated}).',
    },
  },
};
