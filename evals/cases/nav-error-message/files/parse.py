def parse_age(s):
    if not s.isdigit():
        raise ValueError("invalid input")
    return int(s)
