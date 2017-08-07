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
<p>Каждый раз при нажатии на <strong>Получить адрес</strong>, генерируется новый BTC адрес. Один адрес SKY может иметь не более 5 BTC-адресов.</p>
    `,
   
    statusFor: 'Статус по {skyAddress}',
    enterAddress: 'Введите адрес Skycoin',
    getAddress: 'Получить адрес',
    checkStatus: 'Проверить статус',
    loading: 'Загрузка...',
    btcAddress: 'BTC адрес',
    errors: {
      noSkyAddress: 'Пожалуйста введите ваш SKY адрес.',
    },
    statuses: {
      done: 'Транзакция {id}: Skycoin депозит подтверждён (последнее обновление: {updated}).',
      waiting_deposit: 'Транзакция {id}: ожидается BTC депозит (последнее обновление: {updated}).',
      waiting_send: 'Транзакция {id}: BTC депозит получен; Skycoin депозит в очереди (последнее обновление: {updated}).',
      waiting_confirm: 'Транзакция {id}: Skycoin депозит отправлен; ожидаем подтверждение транзакции (последнее обновление: {updated}).',
    }, 
  },
};
