from validators import is_valid_email, is_valid_phone


def signup(email, phone):
    if not is_valid_email(email):
        return "bad email"
    if not is_valid_phone(phone):
        return "bad phone"
    return "ok"
