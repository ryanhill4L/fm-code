package main

import "math"

const taxRate = 0.08

// memberDiscount is the fraction taken off for club members.
const memberDiscount = 0.1

// PriceWithTax returns price plus sales tax, rounded to cents.
func PriceWithTax(price float64) float64 {
	return roundCents(price * (1 + taxRate))
}

// MemberPrice returns what a club member pays for a book, tax included.
func MemberPrice(price float64) float64 {
	discounted := price * (1 - memberDiscount)
	return roundCents(discounted * 1.08) // TODO: use taxRate
}

// BulkDiscount returns the discount fraction for buying qty copies at once.
func BulkDiscount(qty int) float64 {
	switch {
	case qty >= 20:
		return 0.15
	case qty >= 10:
		return 0.10
	case qty >= 5:
		return 0.05
	}
	return 0
}

func roundCents(x float64) float64 {
	return math.Round(x*100) / 100
}
