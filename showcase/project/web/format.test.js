const assert = require("node:assert");
const { formatPrice } = require("./format");

assert.strictEqual(formatPrice(12), "$12.00");
assert.strictEqual(formatPrice(7.5), "$7.50");
assert.strictEqual(formatPrice(9.99), "$9.99");
console.log("format tests passed");
