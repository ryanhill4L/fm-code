package main

import "fmt"

func main() {
	s := NewStore()
	s.Add(User{Name: "ann"})
	fmt.Println(s.Count())
}
