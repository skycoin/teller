// Use Axios for HTTP requests. Refer to https://github.com/mzabriskie/axios
// for usage instructions. If the promises returned by #checkStatus or
// #getAddress reject, they should reject with an Error object containing
// a meaningful error message (will be shown to the user)
//
// export const checkStatus = skyAddress =>
//   axios.get(`https://fake.api/status?address=${skyAddress}`)
//     .catch(() => {
//       throw new Error(`Unable to check status for ${skyAddress}`)
//     });

export const checkStatus = (/* skyAddress */) =>
  Promise.resolve('Transaction pending.');

export const getAddress = (/* skyAddress */) =>
  Promise.resolve('f4keAdr3s5z5x1HUXrCNLbtMDqcw6o5GNn4xqX');
