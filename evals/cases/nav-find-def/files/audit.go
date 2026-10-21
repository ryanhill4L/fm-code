package main

func total(accts []*Account) int {
	t := 0
	for _, a := range accts {
		t += a.Balance
	}
	return t
}
