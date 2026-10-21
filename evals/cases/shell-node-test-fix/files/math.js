function sum(xs) {
  return xs.reduce((a, b) => a + b, 0);
}

function avg(xs) {
  return sum(xs) / xs.length + 1;
}

module.exports = { sum, avg };
