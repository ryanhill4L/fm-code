const test = require('node:test');
const assert = require('node:assert');
const { sum, avg } = require('./math');

test('sum', () => assert.strictEqual(sum([1, 2, 3]), 6));
test('avg', () => assert.strictEqual(avg([2, 4, 6]), 4));
