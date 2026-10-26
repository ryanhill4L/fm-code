package main

import (
	"flag"
	"fmt"
	"os"
)

const version = "0.3.0"

func main() {
	path := flag.String("f", "inventory.json", "inventory file")
	threshold := flag.Int("low", 3, "low-stock threshold")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("bookshop", version)
		return
	}

	books, err := LoadInventory(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bookshop:", err)
		os.Exit(1)
	}

	fmt.Printf("%d titles, total value $%.2f\n", len(books), TotalValue(books))
	for _, b := range LowStock(books, *threshold) {
		fmt.Printf("low stock: %s (%d left)\n", b.Title, b.Quantity)
	}
}
