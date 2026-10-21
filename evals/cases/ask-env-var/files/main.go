package main

import (
	"fmt"
	"os"
)

func main() {
	dir := os.Getenv("FM_DATA_DIR")
	if dir == "" {
		dir = "./data"
	}
	fmt.Println("using", dir)
}
