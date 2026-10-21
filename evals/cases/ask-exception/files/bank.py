class InsufficientFunds(Exception):
    pass


class Account:
    def __init__(self, balance=0):
        self.balance = balance

    def deposit(self, amount):
        self.balance += amount

    def withdraw(self, amount):
        if amount > self.balance:
            raise InsufficientFunds(f"need {amount - self.balance} more")
        self.balance -= amount
