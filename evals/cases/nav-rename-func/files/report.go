package main

func report(us []User) []string {
	var out []string
	for _, u := range us {
		out = append(out, getUserName(u))
	}
	return out
}
