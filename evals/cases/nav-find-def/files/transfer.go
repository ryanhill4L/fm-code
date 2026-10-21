package main

func transfer(from, to *Account, n int) {
	from.Balance -= n
	to.Balance += n
}
