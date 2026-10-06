package main

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/demo"
)

// The demo goes through every page it uses without a hitch, and its
// timetable fits the venue with the panels staffed.
func TestDemoSeed(t *testing.T) {
	if testing.Short() {
		t.Skip("the demo takes a while")
	}
	h := competitionServer(t)
	var out strings.Builder
	if err := demo.Seed(h, &out, 1); err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`Timetable: +(\S+)`).FindStringSubmatch(out.String())
	if m == nil {
		t.Fatalf("no timetable link in:\n%s", out.String())
	}
	page := do(t, h, http.MethodGet, m[1], nil).Body.String()
	for _, want := range []string{"Everything fits.", "HD judge"} {
		if !strings.Contains(page, want) {
			t.Errorf("timetable doesn't say %q", want)
		}
	}
}
