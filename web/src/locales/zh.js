export default {
  distribution: {
    title: '天空币分发活动',
    heading: '天空币分发活动',
    instructions: `
<p>参加天空币分发活动:</p>

<ul>
  <li>在下面输入您的天空币地址</li>
  <li>您将收到一个唯一的比特币地址用来购买天空币</li>
  <li>将比特币发送到您收到的地址上, 您将以每个天空币0.002比特币的价格收到发送的天空币</li>
</ul>

<p>您可以通过输入您的天空币地址并点击下面的"<strong>检查状态</strong>"来核实订单的状态</p>
    `,
    btcAddressFor: '给天空币地址{skyAddress}充值的比特币地址是:',
    statusFor: '天空币地址{skyAddress}的订单状态',
    enterAddress: '输入天空币地址',
    getAddress: '获取地址',
    checkStatus: '检查状态',
    errors: {
      noSkyAddress: '请输入您的天空币地址',
    },
  },
};
