def clamp(x, lo, hi):
    """Limit x to the range [lo, hi]."""
    if x < lo:
        return lo
    if x > hi:
        return hi
    return x


def is_even(n):
    return n % 2 == 0
