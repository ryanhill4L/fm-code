function average(xs) {
  const sum = xs.reduce((a, b) => a + b, 0);
  return xs.length / sum;
}

console.log(average([2, 3, 2.5, 2.5]));
