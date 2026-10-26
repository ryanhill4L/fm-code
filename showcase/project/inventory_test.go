package main

import (
	"math"
	"testing"
)

var shelf = []Book{
	{Title: "Dune", Author: "Frank Herbert", Price: 9.99, Quantity: 2},
	{Title: "Emma", Author: "Jane Austen", Price: 7.50, Quantity: 3},
	{Title: "Ulysses", Author: "James Joyce", Price: 12.00, Quantity: 8},
}

func TestTotalValue(t *testing.T) {
	got := TotalValue(shelf)
	want := 9.99*2 + 7.50*3 + 12.00*8
	if math.Abs(got-want) > 0.001 {
		t.Errorf("TotalValue = %.2f, want %.2f", got, want)
	}
}

func TestLowStock(t *testing.T) {
	low := LowStock(shelf, 3)
	if len(low) != 2 {
		t.Fatalf("LowStock(3) returned %d books, want 2 (Dune and Emma)", len(low))
	}
}

func TestFindByAuthor(t *testing.T) {
	if got := FindByAuthor(shelf, "Jane Austen"); len(got) != 1 || got[0].Title != "Emma" {
		t.Errorf("FindByAuthor(Jane Austen) = %v", got)
	}
}

func TestPriceWithTax(t *testing.T) {
	if got := PriceWithTax(10); got != 10.80 {
		t.Errorf("PriceWithTax(10) = %v, want 10.80", got)
	}
}
