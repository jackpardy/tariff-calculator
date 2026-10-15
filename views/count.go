package views

import "strconv"

// Count is how many of something, in words: "1 entry", "3 entries".
func Count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
