package main

import "fmt"

func main() {
	s := &Stack{}
	s.Push(1)
	s.Push(2)
	s.Push(3)
	fmt.Println(describe(s.Pop()))
}
