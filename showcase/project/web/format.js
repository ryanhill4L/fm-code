// Formatting helpers for the bookshop's web page.

const CURRENCY_SYMBOL = "$";

function formatPrice(amount) {
  return CURRENCY_SYMBOL + amount;
}

function formatStock(qty) {
  if (qty === 0) return "out of stock";
  if (qty === 1) return "1 copy left";
  return qty + " copies left";
}

module.exports = { formatPrice };
