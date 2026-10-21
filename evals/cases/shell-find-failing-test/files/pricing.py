def apply_discount(price, pct):
    return price - price * pct


def with_tax(price):
    return round(price * 1.1, 2)
