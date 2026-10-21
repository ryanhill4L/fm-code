const { TAX_RATE } = require('./tax');

function line(sub) {
  return `Tax (8%): ${Math.round(sub * TAX_RATE)}`;
}

module.exports = { line };
