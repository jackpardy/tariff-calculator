package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"tariffCalculator/store"
)

// checkingFacts is h, failing the test if a request leaves the cache's
// facts (factCache) other than as read: they're shared between requests,
// so a page that changes them shows the next page something not stored.
func checkingFacts(t *testing.T, p *competitionPages, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
		checkFacts(t, p, r.Method+" "+r.URL.Path)
	})
}

// checkFacts compares each competition's cached facts, while still up to
// date, with the same read afresh.
func checkFacts(t *testing.T, p *competitionPages, after string) {
	t.Helper()
	p.facts.mu.Lock()
	cached := map[string]*competitionFacts{}
	for id, f := range p.facts.by {
		cached[id] = f
	}
	p.facts.mu.Unlock()
	for id, f := range cached {
		f.mu.Lock()
		if f.read && f.writes == p.st.Writes(id) {
			var fresh competitionFacts
			if err := fresh.readFrom(context.Background(), p, id); err != nil {
				t.Errorf("after %s: reading competition %s afresh: %v", after, id, err)
			} else if fresh.writes == f.writes {
				if f.judgedIn != nil {
					in, _ := splitWaiting(fresh.all)
					fresh.judgedIn = judge(fresh.c.Competition, in)
				}
				if f.judgedLive != nil {
					fresh.judgedLive = judge(fresh.c.Competition, fresh.rota.entries)
				}
				if f.daysSet {
					fresh.days = daysOf(fresh.c, fresh.rota)
				}
				for _, d := range []struct {
					what        string
					cached, now any
				}{
					{"competition", f.c, fresh.c},
					{"entries", f.all, fresh.all},
					{"rota", f.rota, fresh.rota},
					{"cards", f.judgedIn, fresh.judgedIn},
					{"live cards", f.judgedLive, fresh.judgedLive},
					{"timetable", f.days, fresh.days},
				} {
					if !reflect.DeepEqual(d.cached, d.now) {
						t.Errorf("after %s: the cached %s of competition %s were changed", after, d.what, id)
					}
				}
			}
		}
		f.mu.Unlock()
	}
}

// The cache keeps a competition's facts until anything changes, and reads
// them again after a change made anywhere: by a page, or straight to the
// store.
func TestFactsFollowChanges(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	admin := created(t, s.h, newCompetition())
	c, err := s.st.CompetitionByAdmin(ctx, strings.TrimPrefix(admin, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	aClub(t, s.h, admin, "UCD", true, "Mo")
	shared := func() *store.Entry {
		f, err := s.p.factsOf(ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer f.mu.Unlock()
		return unsafe.SliceData(f.all)
	}
	if first := shared(); first == nil || shared() != first {
		t.Error("with nothing changed, the entries are read once")
	}
	counts := func() (all, live int) {
		entries, err := s.p.entriesOf(ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		f, err := s.p.liveRota(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		return len(entries), len(f.entries)
	}
	if all, live := counts(); all != 1 || live != 1 {
		t.Errorf("%d entries, %d live; want 1 and 1", all, live)
	}
	aClub(t, s.h, admin, "DCU", true, "Ann", "Bo")
	if all, live := counts(); all != 3 || live != 3 {
		t.Errorf("after a club sends two more: %d entries, %d live; want 3 and 3", all, live)
	}
	if err := s.st.SetLimits(ctx, c.ID, map[string]int{"BUCS L3": 1}); err != nil {
		t.Fatal(err)
	}
	if all, live := counts(); all != 3 || live != 1 {
		t.Errorf("with BUCS L3 limited to one: %d entries, %d live; want 3 and 1 (two waiting)", all, live)
	}
	if page := do(t, s.h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(page, "Waiting list") {
		t.Error("the dashboard shows the waiting list at once")
	}

	// What the pages don't read from, and other competitions' changes,
	// keep them.
	entries, err := s.st.Entries(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	other := created(t, s.h, newCompetition())
	o, err := s.st.CompetitionByAdmin(ctx, strings.TrimPrefix(other, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	kept := shared()
	for what, change := range map[string]func() error{
		"a check-in":    func() error { return s.st.SetCheckin(ctx, c.ID, entries[0].ID, store.CheckedIn, "desk") },
		"a notice":      func() error { return s.st.ShowOnScreen(ctx, c.ID, "Lunch", time.Now().Add(time.Hour), "desk") },
		"another's":     func() error { return s.st.SetLimits(ctx, o.ID, map[string]int{"BUCS L3": 5}) },
		"another's too": func() error { return s.st.MarkChecked(ctx, o.ID, "nope", true, "") },
	} {
		_ = change()
		if shared() != kept {
			t.Errorf("after %s, the entries are read again", what)
		}
	}
	if err := s.st.SetLimits(ctx, c.ID, nil); err != nil {
		t.Fatal(err)
	}
	if shared() == kept {
		t.Error("after its own limits change, the entries are read again")
	}
}

// Pages read and changes made at the same time leave the cache right, and
// don't race (go test -race).
func TestFactsUnderLoad(t *testing.T) {
	s := newTestServer(t)
	h := routesWithPages(s.p) // checkingFacts compares as it goes, which needs a quiet store
	admin := created(t, s.h, newCompetition())
	club := aClub(t, s.h, admin, "UCD", true, "Mo")
	day := regexp.MustCompile(`href="(` + regexp.QuoteMeta(club.members["Mo"]) + `/competitions/[^"/]+/day)"`).FindStringSubmatch(memberCompetitionPage(t, s.h, club.members["Mo"]))
	if day == nil {
		t.Fatal("Mo's page links to their competition")
	}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				path := day[1]
				if i%2 == 0 {
					path = admin
				}
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
				if rec.Code != http.StatusOK {
					t.Errorf("%s: %d", path, rec.Code)
					return
				}
			}
		}()
	}
	names := []string{"Ann", "Bo", "Cy", "Di"}
	aClub(t, h, admin, "DCU", true, names...)
	wg.Wait()
	c, err := s.st.CompetitionByAdmin(context.Background(), strings.TrimPrefix(admin, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := s.p.entriesOf(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1+len(names) {
		t.Errorf("%d entries after the load, want %d", len(entries), 1+len(names))
	}
	checkFacts(t, s.p, "the load")
}
