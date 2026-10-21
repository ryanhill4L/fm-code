def is_prime(n):
    return n > 1 and all(n % d for d in range(2, int(n ** 0.5) + 1))


print("sum of primes below 50:", sum(n for n in range(50) if is_prime(n)))
