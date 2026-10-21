def is_valid_phone(s):
    return s.isdigit() and len(s) == 10


def is_valid_email(s):
    return s.count("@") < 2 and " " not in s


def is_valid_zip(s):
    return s.isdigit() and len(s) == 5
