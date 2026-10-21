const { total } = require('./cart');
const { line } = require('./receipt');

console.log(`total ${total([60, 40])}`);
console.log(line(100));
