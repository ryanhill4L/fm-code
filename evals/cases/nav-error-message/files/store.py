DATA = {}


def get(key):
    if key not in DATA:
        raise KeyError("missing key")
    return DATA[key]
