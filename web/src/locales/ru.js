export default {
  distribution: {
    title: 'Skycoin distribution event',
    heading: 'Skycoin distribution event',
    instructions: `
<p>To participate in the distribution event:</p>

<ul>
  <li>Enter your Skycoin address below</li>
  <li>You&apos;ll recieve a unique Bitcoin address to purchase SKY</li>
  <li>Send BTC to the address—you&apos;ll recieve 1 SKY per 0.002 BTC</li>
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
  },
};
