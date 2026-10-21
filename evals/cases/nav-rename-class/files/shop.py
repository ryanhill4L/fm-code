from cart import Cart


def new_cart() -> Cart:
    c = Cart()
    c.add("pen", 3)
    return c
