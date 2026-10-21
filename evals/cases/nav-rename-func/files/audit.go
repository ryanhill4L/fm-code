package main

// audit records who did it. See getUserName for formatting.
func audit(u User) string { return "by " + getUserName(u) }
