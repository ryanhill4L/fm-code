const { TAX_RATE } = require('./tax');

function total(prices) {
  const sub = prices.reduce((a, b) => a + b, 0);
  return Math.round(sub * (1 + TAX_RATE));
}

module.exports = { total };
