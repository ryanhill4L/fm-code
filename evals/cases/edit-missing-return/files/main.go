package main

import "fmt"

func abs(n int) int {
	if n < 0 {
		return -n
	}
	if n >= 0 {
		return n
	}
}

func main() { fmt.Println(abs(-4), abs(7)) }
