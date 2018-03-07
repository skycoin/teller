export default {
  header: {
    navigation: {
      whitepapers: 'Whitepapers',
      downloads: 'Downloads',
      explorer: 'Explorer',
      blog: 'Blog',
      roadmap: 'Roadmap',
    },
  },
  footer: {
    getStarted: 'Get started',
    explore: 'Explore',
    community: 'Community',
    wallet: 'Get Wallet',
    infographics: 'Infographics',
    whitepapers: 'Whitepapers',
    blockchain: 'Blockchain Explorer',
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
    rate: 'Current OTC rate: {rate} MDL/BTC',
    inventory: 'Current inventory: {coins} MDL available',
    title: 'MDL OTC',
    heading: 'MDL OTC',
    headingEnded: 'MDL OTC is currently closed',
    ended: `<p>Join the <a href="https://t.me/mdl">MDL Telegram</a>
      or follow the
      <a href="https://twitter.com/mdlproject">MDL Twitter</a>.`,
    instructions: `<p>You can check the current market value for <a href="https://coinmarketcap.com/currencies/mdl/">MDL at CoinMarketCap</a>.</p>

<p>To use the MDL OTC:</p>

<ul>
  <li>Enter your MDL address below</li>
  <li>You&apos;ll receive a unique Bitcoin address to purchase MDL</li>
  <li>Send BTC to the address</li>
</ul>

<p>You can check the status of your order by entering your address and selecting <strong>Check status</strong>.</p>
<p>Each time you select <strong>Get Address</strong>, a new BTC address is generated. A single MDL address can have up to 5 BTC addresses assigned to it.</p>
    `,
    statusFor: 'Status for {mdlAddress}',
    enterAddress: 'Enter MDL address',
    getAddress: 'Get address',
    checkStatus: 'Check status',
    loading: 'Loading...',
    btcAddress: 'BTC address',
    errors: {
      noSkyAddress: 'Please enter your MDL address.',
      coinsSoldOut: 'MDL OTC is currently sold out, check back later.',
    },
    statuses: {
      waiting_deposit: '[tx-{id} {updated}] Waiting for BTC deposit.',
      waiting_send: '[tx-{id} {updated}] BTC deposit confirmed. MDL transaction is queued.',
      waiting_confirm: '[tx-{id} {updated}] MDL transaction sent.  Waiting to confirm.',
      done: '[tx-{id} {updated}] Completed. Check your MDL wallet.',
    },
  },
};
