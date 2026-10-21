package main

import "fmt"

func main() {
	a, b := &Account{1, 10}, &Account{2, 5}
	transfer(a, b, 3)
	fmt.Println(total([]*Account{a, b}))
}
