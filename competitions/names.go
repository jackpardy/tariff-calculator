package competitions

import (
	"slices"
	"strings"
	"unicode"
)

// Names that look alike (ADR 0005, Consequences): the timetable matches
// people by member link, by entry or by an individual's name, so the same
// gymnast entered twice under slightly different names would be scheduled
// as two people. These are flagged for the organiser to look at.

// fold is a name reduced to its letters, lower case, without accents:
// "Dara O'Néill" → "daraoneill".
func fold(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if base, ok := plain[r]; ok {
			r = base
		}
		if unicode.IsLetter(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// plain is each accented letter's plain letter, for the letters of the
// languages a club's members are likely to write their names in.
var plain = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ä': 'a', 'ã': 'a', 'å': 'a', 'ą': 'a', 'ă': 'a', 'ā': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e', 'ę': 'e', 'ě': 'e', 'ė': 'e', 'ē': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i', 'ī': 'i', 'į': 'i', 'ı': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'ö': 'o', 'õ': 'o', 'ø': 'o', 'ő': 'o', 'ō': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u', 'ů': 'u', 'ű': 'u', 'ū': 'u', 'ų': 'u',
	'ý': 'y', 'ÿ': 'y', 'ç': 'c', 'ć': 'c', 'č': 'c', 'ď': 'd', 'đ': 'd', 'ğ': 'g',
	'ł': 'l', 'ľ': 'l', 'ĺ': 'l', 'ñ': 'n', 'ń': 'n', 'ň': 'n', 'ř': 'r', 'ŕ': 'r',
	'ś': 's', 'š': 's', 'ş': 's', 'ș': 's', 'ť': 't', 'ţ': 't', 'ț': 't',
	'ź': 'z', 'ż': 'z', 'ž': 'z',
}

// sortKey is a name for sorting: lower case, without accents or
// punctuation, but keeping its spaces and digits, so "Ó Briain" sorts
// with the O's.
func sortKey(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if base, ok := plain[r]; ok {
			r = base
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CompareNames orders names as people expect to find them in a list,
// ignoring case and accents: "Aberdeen" before "ATU Sligo", "Éabha" before
// "Tadhg". Names that differ only in those come in byte order.
func CompareNames(a, b string) int {
	if c := strings.Compare(sortKey(a), sortKey(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

// distance is the edit distance between two strings, by rune.
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// LookAlike are pairs of different people whose names look like the same
// person's: the same letters (ignoring case, spaces, punctuation and
// accents), or one letter apart in a name of ten letters or more. names
// are people's names, by key; each pair is sorted, and the pairs too.
func LookAlike(names map[string]string) [][2]string {
	var keys []string
	for k := range names {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var out [][2]string
	for i, a := range keys {
		fa := fold(names[a])
		for _, b := range keys[i+1:] {
			fb := fold(names[b])
			if fa == "" || fb == "" {
				continue
			}
			if fa == fb || (min(len(fa), len(fb)) >= 10 && distance(fa, fb) <= 1) {
				pair := [2]string{names[a], names[b]}
				if pair[1] < pair[0] {
					pair[0], pair[1] = pair[1], pair[0]
				}
				out = append(out, pair)
			}
		}
	}
	slices.SortFunc(out, func(x, y [2]string) int { return strings.Compare(x[0]+x[1], y[0]+y[1]) })
	return out
}

// NameMatches says whether a name matches a search: each of the search's
// words is in the name, ignoring case, accents and punctuation ("o'neill"
// finds "Dara O'Néill", "dar nei" too).
func NameMatches(name, search string) bool {
	folded := fold(name)
	for _, word := range strings.Fields(search) {
		if !strings.Contains(folded, fold(word)) {
			return false
		}
	}
	return true
}
