package main

// Sums the prices.
func calc(prices []int) int {
	t := 0
	for _, p := range prices {
		t += p
	}
	return t
}
