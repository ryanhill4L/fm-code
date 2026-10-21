from cart import Cart


def checkout(cart: Cart):
    return f"paid {cart.total()}"
