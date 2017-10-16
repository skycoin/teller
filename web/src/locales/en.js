export default {
  header: {
    navigation: {
      distribution: 'Distribution',
      distributionEvent: 'Distribution event',
      whitepapers: 'Whitepapers',
      downloads: 'Downloads',
      explorer: 'Explorer',
      blog: 'Blog',
      buy: 'Buy Skycoin',
      roadmap: 'Roadmap',
    },
  },
  footer: {
    getStarted: 'Get started',
    explore: 'Explore',
    community: 'Community',
    wallet: 'Get Wallet',
    buy: 'Buy Skycoin',
    infographics: 'Infographics',
    whitepapers: 'Whitepapers',
    blockchain: 'Blockchain Explorer',
    distribution: 'Distribution',
    blog: 'Blog',
    twitter: 'Twitter',
    reddit: 'Reddit',
    github: 'Github',
    telegram: 'Telegram',
    slack: 'Slack',
    roadmap: 'Roadmap',
    skyMessenger: 'Sky-Messenger',
    cxPlayground: 'CX Playground',
    team: 'Team',
    subscribe: 'Mailing List',
    market: 'Markets',
    bitcoinTalks: 'Bitcointalks ANN',
    instagram: 'Instagram',
    facebook: 'Facebook',
    discord: 'Discord',
  },
  distribution: {
    title: 'Skycoin distribution event',
    heading: 'Skycoin distribution event',
    headingEnded: 'The previous distribution event finished on',
    ended: `<p>Join the <a href="https://t.me/skycoin">Skycoin Telegram</a>, 
      <a href="https://skycoin.slack.com">Skycoin Slack</a> or follow the 
      <a href="https://twitter.com/skycoinproject">Skycoin Twitter</a> 
      to learn when the next event begins.`,
    instructions: `
<p>To participate in the distribution event:</p>

<ul>
  <li>Enter your Skycoin address below</li>
  <li>You&apos;ll receive a unique Bitcoin address to purchase SKY</li>
  <li>Send BTC to the address—you&apos;ll receive 1 SKY per 0.002 BTC</li>
  <li><strong>Only send a multiple of 0.002BTC. You must send at least 0.002BTC. SKY is sent in whole numbers; fractional SKY is not sent.</strong></li>
</ul>

<p>You can check the status of your order by entering your address and selecting <strong>Check status</strong>.</p>
<p>Each time you select <strong>Get Address</strong>, a new BTC address is generated. A single SKY address can have up to 5 BTC addresses assigned to it.</p>
    `,
    statusFor: 'Status for {skyAddress}',
    enterAddress: 'Enter Skycoin address',
    getAddress: 'Get address',
    checkStatus: 'Check status',
    loading: 'Loading...',
    btcAddress: 'BTC address',
    errors: {
      noSkyAddress: 'Please enter your SKY address.',
    },
    statuses: {
      waiting_deposit: '[tx-{id} {updated}] Waiting for BTC deposit.',
      waiting_send: '[tx-{id} {updated}] BTC deposit confirmed. Skycoin transaction is queued.',
      waiting_confirm: '[tx-{id} {updated}] Skycoin transaction sent.  Waiting to confirm.',
      done: '[tx-{id} {updated}] Completed. Check your Skycoin wallet.',
    },
  },
};
