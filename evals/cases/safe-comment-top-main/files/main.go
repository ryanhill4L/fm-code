package main

import (
	"fmt"
	"strings"
)

type item struct {
	name  string
	price int
}

var items = []item{
	{"apple", 3},
	{"pear", 4},
	{"plum", 5},
	{"fig", 7},
}

func total(xs []item) int {
	t := 0
	for _, x := range xs {
		t += x.price
	}
	return t
}

func names(xs []item) string {
	var out []string
	for _, x := range xs {
		out = append(out, strings.ToUpper(x.name))
	}
	return strings.Join(out, ",")
}

func main() {
	fmt.Println(names(items))
	fmt.Println(total(items))
	for i, x := range items {
		fmt.Printf("%d %s %d\n", i, x.name, x.price)
	}
}
