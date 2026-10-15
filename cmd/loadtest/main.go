// Command loadtest acts as the people at a competition, to measure how the
// app holds up before the dry run (roadmap). It reads a list of pages, one
// person's a line as "kind path", which `tariff demo -pages FILE` writes for
// the demo, and runs three things at once against a test copy:
//
//   - a burst: every person opens their page within a few seconds, as after
//     a desk message or a push to everyone;
//   - then steady use: each opens it again every so often, the staff keep
//     the organiser's On the day page open, and the venue screens refresh
//     every 30 seconds, as they do;
//   - the marshals' taps: a gymnast checked in, or cleared again, every few
//     seconds, so the pages' cache is refreshed as it would be on the day.
//
// It then says, for each kind of page, how many loads there were, how many
// failed and how long they took. Never point it at the live site: the taps
// change the competition.
//
//	go run ./cmd/loadtest -base https://test.example -pages pages.txt -people 300
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

func main() {
	base := flag.String("base", "http://localhost:8080", "the test copy's address")
	pagesFile := flag.String("pages", "pages.txt", "the pages, one a line as \"kind path\" (tariff demo -pages)")
	people := flag.Int("people", 300, "how many people open their own page")
	burst := flag.Duration("burst", 10*time.Second, "how long everyone takes to open their page at first")
	steady := flag.Duration("steady", 2*time.Minute, "how long steady use goes on after the burst")
	think := flag.Duration("think", 30*time.Second, "how long, on average, each person waits before opening their page again")
	staff := flag.Int("staff", 6, "how many keep the organiser's On the day page open, reloading it every 10 seconds")
	screens := flag.Int("screens", 7, "how many venue screens refresh every 30 seconds")
	taps := flag.Duration("taps", 5*time.Second, "how often a marshal taps a check-in (0: never)")
	flag.Parse()

	pages, err := readPages(*pagesFile)
	if err != nil {
		log.Fatal(err)
	}
	var personal []page // gymnasts', coaches' and clubs' own pages
	for _, p := range pages {
		if p.kind != "organiser" && p.kind != "day" && p.kind != "screen" {
			personal = append(personal, p)
		}
	}
	day, okDay := first(pages, "day")
	screen, okScreen := first(pages, "screen")
	if len(personal) == 0 || !okDay || !okScreen {
		log.Fatalf("%s needs people's pages, a day page and a screen", *pagesFile)
	}
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 1))
	rng.Shuffle(len(personal), func(i, j int) { personal[i], personal[j] = personal[j], personal[i] })
	for len(personal) < *people {
		personal = append(personal, personal...) // more people than pages: some share
	}
	personal = personal[:*people]

	t := &tester{base: strings.TrimSuffix(*base, "/"), stats: map[string]*stat{}, client: &http.Client{
		Timeout:       30 * time.Second,
		Transport:     &http.Transport{MaxIdleConnsPerHost: *people + *staff + *screens + 2},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
	ctx, stop := context.WithTimeout(context.Background(), *burst+*steady)
	defer stop()
	start := time.Now()
	fmt.Printf("%d people, %d staff on On the day, %d screens, a tap every %s, against %s: a %s burst, then %s steady.\n",
		*people, *staff, *screens, *taps, t.base, *burst, *steady)

	var wg sync.WaitGroup
	every := func(n int, p page, period time.Duration) {
		for i := range n {
			wg.Go(func() {
				sleep(ctx, time.Duration(i)*period/time.Duration(n)) // spread them out
				for ctx.Err() == nil {
					t.get(ctx, p)
					sleep(ctx, period)
				}
			})
		}
	}
	for i, p := range personal {
		seed := uint64(i)
		wg.Go(func() {
			r := rand.New(rand.NewPCG(seed, 2))
			sleep(ctx, time.Duration(r.Int64N(int64(*burst))))
			t.get(ctx, page{kind: p.kind + " (burst)", path: p.path})
			for ctx.Err() == nil {
				// Waits spread around the average, none under a tenth of it.
				sleep(ctx, *think/10+time.Duration(r.ExpFloat64()*float64(*think*9/10)))
				if ctx.Err() == nil {
					t.get(ctx, p)
				}
			}
		})
	}
	every(*staff, day, 10*time.Second)
	every(*screens, screen, 30*time.Second)
	if *taps > 0 {
		wg.Go(func() { t.tapping(ctx, day, *taps, rng) })
	}
	wg.Wait()
	t.report(os.Stdout, time.Since(start))
}

// page is one page someone opens: its kind (member, coach, club, day and so
// on) and path.
type page struct{ kind, path string }

func readPages(name string) ([]page, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []page
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		if kind, path, ok := strings.Cut(strings.TrimSpace(lines.Text()), " "); ok {
			out = append(out, page{kind, path})
		}
	}
	return out, lines.Err()
}

func first(pages []page, kind string) (page, bool) {
	i := slices.IndexFunc(pages, func(p page) bool { return p.kind == kind })
	if i < 0 {
		return page{}, false
	}
	return pages[i], true
}

// sleep waits for d, or until ctx is done.
func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

type tester struct {
	base   string
	client *http.Client
	mu     sync.Mutex
	stats  map[string]*stat
}

// stat is how the loads of one kind of page went.
type stat struct {
	times  []time.Duration // of those that worked
	failed int
	why    map[string]int // what went wrong, and how often
}

// get loads a page, as a browser would, and notes how long it took.
func (t *tester) get(ctx context.Context, p page) {
	t.do(ctx, p.kind, http.MethodGet, p.path, nil)
}

func (t *tester) do(ctx context.Context, kind, method, path string, form url.Values) string {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, t.base+path, body)
	if err != nil {
		log.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	began := time.Now()
	res, err := t.client.Do(req)
	var page []byte
	if err == nil {
		page, err = io.ReadAll(res.Body)
		res.Body.Close()
		if err == nil && res.StatusCode >= 400 {
			err = errors.New(res.Status)
		}
	}
	took := time.Since(began)
	if ctx.Err() != nil {
		return "" // cut off at the end: not the server's doing
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.stats[kind]
	if s == nil {
		s = &stat{why: map[string]int{}}
		t.stats[kind] = s
	}
	if err != nil {
		s.failed++
		s.why[strings.ReplaceAll(err.Error(), t.base+path, "the page")]++ // no secret link in the report
		return ""
	}
	s.times = append(s.times, took)
	return string(page)
}

var checkinForm = regexp.MustCompile(`name="entry" value="([^"]+)"`)

// tapping checks a gymnast in, or clears them, every so often, as the
// marshals do, on the organiser's On the day page.
func (t *tester) tapping(ctx context.Context, day page, every time.Duration, rng *rand.Rand) {
	var entries []string
	for _, m := range checkinForm.FindAllStringSubmatch(t.do(ctx, "day", http.MethodGet, day.path, nil), -1) {
		if !slices.Contains(entries, m[1]) {
			entries = append(entries, m[1])
		}
	}
	if len(entries) == 0 {
		log.Print("No check-in on the day page to tap: the timetable may not be published.")
		return
	}
	here := map[string]bool{}
	for ctx.Err() == nil {
		sleep(ctx, every)
		e := entries[rng.IntN(len(entries))]
		status := "here"
		if here[e] {
			status = ""
		}
		here[e] = !here[e]
		t.do(ctx, "tap (check-in)", http.MethodPost, day.path+"/checkin", url.Values{"entry": {e}, "status": {status}})
	}
}

// report says how each kind of page held up.
func (t *tester) report(out io.Writer, took time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	kinds := make([]string, 0, len(t.stats))
	total, failed := 0, 0
	for k, s := range t.stats {
		kinds = append(kinds, k)
		total += len(s.times) + s.failed
		failed += s.failed
	}
	slices.Sort(kinds)
	fmt.Fprintf(out, "\n%d loads in %s (%.1f a second), %d failed.\n\n", total, took.Round(time.Second), float64(total)/took.Seconds(), failed)
	fmt.Fprintf(out, "%-20s %7s %7s %9s %9s %9s %9s\n", "Page", "Loads", "Failed", "Median", "90%", "99%", "Slowest")
	for _, k := range kinds {
		s := t.stats[k]
		slices.Sort(s.times)
		at := func(q float64) string {
			if len(s.times) == 0 {
				return "-"
			}
			return s.times[min(len(s.times)-1, int(q*float64(len(s.times))))].Round(time.Millisecond).String()
		}
		fmt.Fprintf(out, "%-20s %7d %7d %9s %9s %9s %9s\n", k, len(s.times)+s.failed, s.failed, at(0.5), at(0.9), at(0.99), at(1))
	}
	for _, k := range kinds {
		for why, n := range t.stats[k].why {
			fmt.Fprintf(out, "  %s: %s (%d)\n", k, why, n)
		}
	}
}
