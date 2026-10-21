package main

import "fmt"

func main() {
	u := User{"Ada", "Lovelace"}
	fmt.Println(greet(u), report([]User{u}), audit(u))
}
