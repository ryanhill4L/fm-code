package main

type User struct{ First, Last string }

func getUserName(u User) string { return u.First + " " + u.Last }
