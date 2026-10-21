const { addTax } = require("./tax");

function total(items) {
  const subtotal = items.reduce((sum, i) => sum + i.price * i.qty, 0);
  return addTax(subtotal);
}

module.exports = { total };
