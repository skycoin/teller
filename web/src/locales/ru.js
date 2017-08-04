export default {
  distribution: {
    title: 'Распространение Skycoin',
    heading: 'Распространение Skycoin',
    instructions: `
<p>Что необходимо для участия в распространении:</p>

<ul>
  <li>Введите ваш Skycoin адрес</li>
  <li>Вы получите уникальный Bitcoin адрес для приобретения SKY</li>
  <li>Пошлите Bitcoin на полученый адрес: 1 SKY стоит 0.002 BTC</li>
</ul>

<p>Вы можете проверить статус заказа, введя адрес SKY и нажав на <strong>Проверить статус</strong>.</p>
    `,
    btcAddressFor: 'BTC адрес для {skyAddress}',
    statusFor: 'Статус по {skyAddress}',
    enterAddress: 'Введите адрес Skycoin',
    getAddress: 'Получить адрес',
    checkStatus: 'Проверить статус',
    errors: {
      noSkyAddress: 'Пожалуйста введите ваш SKY адрес.',
    },
  },
};
