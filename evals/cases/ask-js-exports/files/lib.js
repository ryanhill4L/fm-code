function pad(s, n) {
  return s.padEnd(n, " ");
}

function slugify(s) {
  return s.toLowerCase().replace(/\s+/g, "-");
}

function truncate(s, n) {
  return s.length > n ? s.slice(0, n) + "..." : s;
}

module.exports = { slugify, truncate };
