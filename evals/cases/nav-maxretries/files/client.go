package main

func attempt(n int) bool { return n >= 100 }

func Run() int {
	tries := 0
	for i := 0; i < MaxRetries; i++ {
		tries++
		if attempt(i) {
			break
		}
	}
	return tries
}
