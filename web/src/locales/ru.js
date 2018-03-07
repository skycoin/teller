export default {
  header: {
    navigation: {
      whitepapers: 'Документация',
      downloads: 'Загрузки',
      explorer: 'Обозреватель',
      blog: 'Блог',
      roadmap: 'План',
    },
  },
  footer: {
    getStarted: 'Начать',
    explore: 'Дополнительно',
    community: 'Сообщество',
    wallet: 'Получить Кошелёк',
    infographics: 'Инфографика',
    whitepapers: 'Документация',
    blockchain: 'Блокчейн Обозреватель',
    blog: 'Блог',
    twitter: 'Twitter',
    reddit: 'Reddit',
    github: 'Github',
    telegram: 'Telegram',
    slack: 'Slack',
    roadmap: 'План',
    skyMessenger: 'Sky-Messenger',
    cxPlayground: 'CX Playground',
    team: 'Команда',
    subscribe: 'Рассылка',
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
    headingEnded: 'The previous distribution event finished on',
    ended: `<p>Join the <a href="https://t.me/mdl">MDL Telegram</a>
      or follow the
      <a href="https://twitter.com/mdlproject">MDL Twitter</a>
      to learn when the next event begins.`,
    instructions: `<p>You can check the current market value for <a href="https://coinmarketcap.com/currencies/mdl/">MDL at CoinMarketCap</a>.</p>

<p>Что необходимо для участия в распространении:</p>

<ul>
  <li>Введите ваш MDL адрес</li>
  <li>Вы получите уникальный Bitcoin адрес для приобретения MDL</li>
  <li>Пошлите Bitcoin на полученый адрес</li>
</ul>

<p>Вы можете проверить статус заказа, введя адрес MDL и нажав на <strong>Проверить статус</strong>.</p>
<p>Каждый раз при нажатии на <strong>Получить адрес</strong>, генерируется новый BTC адрес. Один адрес MDL может иметь не более 5 BTC-адресов.</p>
    `,
    statusFor: 'Статус по {mdlAddress}',
    enterAddress: 'Введите адрес MDL',
    getAddress: 'Получить адрес',
    checkStatus: 'Проверить статус',
    loading: 'Загрузка...',
    btcAddress: 'BTC адрес',
    errors: {
      noSkyAddress: 'Пожалуйста введите ваш MDL адрес.',
      coinsSoldOut: 'MDL OTC is currently sold out, check back later.',
    },
    statuses: {
      waiting_deposit: '[tx-{id} {updated}] Ожидаем BTC депозит.',
      waiting_send: '[tx-{id} {updated}] BTC депозит подтверждён. MDL транзакция поставлена в очередь.',
      waiting_confirm: '[tx-{id} {updated}] MDL транзакция отправлена. Ожидаем подтверждение.',
      done: '[tx-{id} {updated}] Завершена. Проверьте ваш MDL кошелёк.',
    },
  },
};
