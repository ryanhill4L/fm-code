package main

import "testing"

func TestIsEven(t *testing.T) {
	if !isEven(4) || !isEven(0) {
		t.Fatal("4 and 0 should be even")
	}
	if isEven(7) {
		t.Fatal("7 should not be even")
	}
}
