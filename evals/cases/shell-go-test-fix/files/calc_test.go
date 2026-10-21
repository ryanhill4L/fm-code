package main

import "testing"

func TestDouble(t *testing.T) {
	if Double(4) != 8 {
		t.Fatal("Double(4) != 8")
	}
}

func TestSquare(t *testing.T) {
	if Square(5) != 25 {
		t.Fatal("Square(5) != 25")
	}
}

func TestNegate(t *testing.T) {
	if Negate(3) != -3 {
		t.Fatal("Negate(3) != -3")
	}
}
