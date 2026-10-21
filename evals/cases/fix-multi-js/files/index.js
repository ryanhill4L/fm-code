const { total } = require("./cart");
const { money } = require("./format");

const items = [
  { price: 20, qty: 2 },
  { price: 60, qty: 1 },
];
console.log("total: " + money(total(items)));
