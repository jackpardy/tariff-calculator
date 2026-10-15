package web

import (
	"context"
	"flag"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/demo"
)

// fullDemo runs TestDemoSeed on the whole demo, as it's shown, rather than
// a few clubs: go test ./web -run TestDemoSeed -full.
var fullDemo = flag.Bool("full", false, "seed the whole demo in TestDemoSeed, not a few clubs")

// The demo goes through every page it uses without a hitch: its clubs are
// entered, every gymnast in it is in a flight (but those on a waiting
// list), and the rota seats its panels.
func TestDemoSeed(t *testing.T) {
	size := demo.Small
	if *fullDemo {
		size = demo.Full
	}
	s := newTestServer(t)
	var out strings.Builder
	if err := demo.SeedSize(s.h, &out, 1, size); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^(Organiser \(dashboard\)|Timetable): +(\S+)$`).FindAllStringSubmatch(out.String(), -1) {
		links[m[1]] = m[2]
	}
	admin, tt := links["Organiser (dashboard)"], links["Timetable"]
	if admin == "" || tt == "" {
		t.Fatalf("no organiser or timetable link in:\n%s", out.String())
	}

	ctx := context.Background()
	c, err := s.st.CompetitionByAdmin(ctx, strings.TrimPrefix(admin, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	clubs, err := s.st.CompetitionClubs(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(clubs) != size.Clubs {
		t.Errorf("%d clubs entered, want %d", len(clubs), size.Clubs)
	}
	all, err := s.st.Entries(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	entries, waiting := live(all), 0 // live leaves out those waiting
	for _, e := range all {
		if !e.Withdrawn && e.Waiting > 0 {
			waiting++
		}
	}
	// Each club's members enter trampoline (about 30 each), and six
	// gymnasts enter on their own.
	if min := 20*size.Clubs + 6; len(entries) < min {
		t.Errorf("%d entries, want at least %d", len(entries), min)
	}
	if c.Published == nil {
		t.Fatal("the timetable isn't published")
	}
	inFlights, seats, filled := 0, 0, 0
	for _, f := range c.Published.Flights {
		inFlights += len(f.Entries)
		for _, d := range f.Officials {
			seats++
			if d.Person != "" {
				filled++
			}
		}
	}
	if len(c.Published.Flights) < 10 || inFlights != len(entries) {
		t.Errorf("%d flights hold %d entries; want every one of %d not waiting", len(c.Published.Flights), inFlights, len(entries))
	}
	want := seats * 3 / 4 // a few clubs bring too few judges to fill every seat
	if *fullDemo {
		want = seats * 19 / 20
	}
	if seats == 0 || filled < want {
		t.Errorf("%d of %d rota seats filled, want at least %d", filled, seats, want)
	}
	if *fullDemo {
		t.Run("waiting lists", func(t *testing.T) {
			if waiting == 0 {
				t.Error("the demo fills BUCS L1 and DMT Advanced, with a waiting list")
			}
		})
		page := do(t, s.h, http.MethodGet, tt, nil).Body.String()
		for _, want := range []string{"Everything fits.", "HD judge"} {
			if !strings.Contains(page, want) {
				t.Errorf("timetable doesn't say %q", want)
			}
		}
	}
	t.Logf("%d clubs, %d entries (%d waiting), %d flights, %d of %d seats filled", len(clubs), len(entries), waiting, len(c.Published.Flights), filled, seats)
}
