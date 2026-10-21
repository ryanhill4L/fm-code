def check_token(t):
    if t != "secret":
        raise PermissionError("invalid token")
    return True
