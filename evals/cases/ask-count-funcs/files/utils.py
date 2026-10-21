def clamp(x, lo, hi):
    return max(lo, min(x, hi))


def is_blank(s):
    return not s.strip()


def first(xs, default=None):
    return xs[0] if xs else default


def flatten(xss):
    return [x for xs in xss for x in xs]


def chunk(xs, n):
    return [xs[i:i + n] for i in range(0, len(xs), n)]
