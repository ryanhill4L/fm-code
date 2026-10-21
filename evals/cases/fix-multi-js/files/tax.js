const RATE = 0.1;

function addTax(amount) {
  return amount + (amount * RATE) / 10;
}

module.exports = { addTax };
