package competitions

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestLookAlike(t *testing.T) {
	got := LookAlike(map[string]string{
		"i:dara o'néill": "Dara O'Néill", "i:dara oneill": "Dara ONeill", // accents and punctuation
		"m:1": "Siobhan Murphy", "i:siobhan murphey": "Siobhan Murphey", // one letter apart
		"m:2": "Ann Lee", "m:3": "Anne Lee", // too short to call
		"m:4": "Tom Kelly", "m:5": "Tim Kelly",
	})
	if fmt.Sprint(got) != "[[Dara O'Néill Dara ONeill] [Siobhan Murphey Siobhan Murphy]]" {
		t.Errorf("look alike: %v", got)
	}
}

// lookAlikeAsWas is LookAlike as it was, every pair compared by edit
// distance, to check the faster one finds the same.
func lookAlikeAsWas(names map[string]string) [][2]string {
	distance := func(a, b string) int {
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

// LookAlike finds what comparing every pair by edit distance did.
func TestLookAlikeAsWas(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	first := []string{"Siobhan", "Siobhán", "Aoife", "Ciara", "Dara", "Éabha", "Tadhg", "Niamh", "Seán", "Sean", "Ann", "Anne", "Ζωή", "Łucja"}
	last := []string{"Murphy", "Murphey", "O'Néill", "ONeill", "O Neill", "Kelly", "Kelley", "Byrne", "Byrnes", "Mac Giolla Phádraig", "Ní Bhriain", "Lee", ""}
	for range 50 {
		names := map[string]string{}
		for i := range 40 {
			names[fmt.Sprint(i)] = first[r.IntN(len(first))] + " " + last[r.IntN(len(last))]
		}
		names["empty"], names["punctuation"] = "", "?!"
		if got, want := LookAlike(names), lookAlikeAsWas(names); !reflect.DeepEqual(got, want) {
			t.Fatalf("%v: got %v, want %v", names, got, want)
		}
	}
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"abc", "abc", true}, {"abc", "abd", true}, {"abc", "abcd", true}, {"abc", "xabc", true},
		{"abc", "ab", true}, {"abc", "axc", true}, {"abc", "acb", false}, {"abc", "abde", false}, {"", "a", true}, {"", "ab", false},
	} {
		if got := withinOne([]rune(c.a), []rune(c.b)); got != c.want {
			t.Errorf("withinOne(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// 400 gymnasts, as at ISTO: go test -bench LookAlike ./competitions.
func BenchmarkLookAlike(b *testing.B) {
	r := rand.New(rand.NewPCG(1, 2))
	names := map[string]string{}
	for i := range 400 {
		var n []byte
		for range 8 + r.IntN(10) {
			n = append(n, byte('a'+r.IntN(26)))
		}
		names[fmt.Sprint(i)] = string(n[:4]) + " " + string(n[4:])
	}
	for b.Loop() {
		LookAlike(names)
	}
}

func TestNameMatches(t *testing.T) {
	for _, c := range []struct {
		name, search string
		want         bool
	}{
		{"Dara O'Néill", "o'neill", true},
		{"Dara O'Néill", "dar nei", true},
		{"Dara O'Néill & Ann Ryan", "ryan", true},
		{"Dara O'Néill", "ryan", false},
		{"Dara O'Néill", "", true},
		{"Mary Kate Byrne", "marykate", true},
		{"Tomáš Novák", "tomas novak", true},
		{"Zuzana Rýdlová", "rydlova", true},
	} {
		if got := NameMatches(c.name, c.search); got != c.want {
			t.Errorf("%q in %q: %v", c.search, c.name, got)
		}
	}
}

func TestCompareNames(t *testing.T) {
	got := [][]string{
		{"ATU Sligo", "Aberdeen", "DkIT", "DCU"},
		{"TUS Athlone", "Trinity", "TU Dublin"},
		{"Tadhg Ryan", "Éabha Kelly", "aoife Byrne", "Ó Briain"},
	}
	want := []string{
		"[Aberdeen ATU Sligo DCU DkIT]",
		"[Trinity TU Dublin TUS Athlone]",
		"[aoife Byrne Éabha Kelly Ó Briain Tadhg Ryan]",
	}
	for i, names := range got {
		slices.SortFunc(names, CompareNames)
		if fmt.Sprint(names) != want[i] {
			t.Errorf("sorted: %v, want %s", names, want[i])
		}
	}
}
