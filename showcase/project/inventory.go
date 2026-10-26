package main

import (
	"encoding/json"
	"os"
)

// Book is one title on the shelf.
type Book struct {
	Title    string  `json:"title"`
	Author   string  `json:"author"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}

// LoadInventory reads the book list from a JSON file.
func LoadInventory(path string) ([]Book, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var books []Book
	err = json.Unmarshal(data, &books)
	return books, err
}

func TotalValue(books []Book) float64 {
	total := 0.0
	for _, b := range books {
		total += b.Price * float64(b.Quantity)
	}
	return total
}

// LowStock returns the books with at most threshold copies left.
func LowStock(books []Book, threshold int) []Book {
	var low []Book
	for _, b := range books {
		if b.Quantity < threshold {
			low = append(low, b)
		}
	}
	return low
}

// FindByAuthor returns the books written by author.
// TODO: match authors case-insensitively.
func FindByAuthor(books []Book, author string) []Book {
	var found []Book
	for _, b := range books {
		if b.Author == author {
			found = append(found, b)
		}
	}
	return found
}

func titles(books []Book) []string {
	var out []string
	for _, b := range books {
		out = append(out, b.Title)
	}
	return out
}
