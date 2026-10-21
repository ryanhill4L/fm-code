from db import connect
from models import User


def main():
    conn = connect()
    print(User("ann"), conn)


if __name__ == "__main__":
    main()
