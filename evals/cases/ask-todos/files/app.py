import urllib.request


def fetch(url):
    # TODO: add retry on network error
    with urllib.request.urlopen(url) as r:
        return r.read()


def lookup(key, store):
    # TODO: cache results between calls
    return store.get(key)


def register(email):
    # TODO: validate the email address
    return {"email": email}


def main():
    # entry point
    print(register("a@b.c"))
