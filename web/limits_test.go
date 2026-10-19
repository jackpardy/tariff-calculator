package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestLimitsAndWaitingLists(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	var links []string
	for _, name := range []string{"Ann Ryan", "Bea Kelly", "Cat Doyle"} {
		links = append(links, withoutQuery(redirected(t, h, enter, bucsEntry(name))))
	}
	if loc := redirected(t, h, admin+"/limits", url.Values{"limit-0": {"two"}}); !strings.Contains(loc, "Nothing+was+changed") {
		t.Errorf("a limit must be a number: %s", loc)
	}
	// BUCS L3 is the first event; it takes 2. That moves Cat, the last in,
	// to the waiting list: it asks first, saying so.
	ask := do(t, h, http.MethodPost, admin+"/limits", url.Values{"limit-0": {"2"}})
	if body := ask.Body.String(); ask.Code != http.StatusOK || !strings.Contains(body, "This moves 1 entry in now to a waiting list.") ||
		!strings.Contains(body, "To the waiting list: Cat Doyle (On their own), BUCS L3") || !strings.Contains(body, `<input type="hidden" name="anyway" value="1">`) {
		t.Fatalf("asks before moving Cat to the waiting list: %s", body)
	}
	if strings.Contains(do(t, h, http.MethodGet, admin, nil).Body.String(), "2 of 2 places") {
		t.Error("nothing changes until it's saved anyway")
	}
	if loc := redirected(t, h, admin+"/limits", url.Values{"limit-0": {"2"}, "anyway": {"1"}}); !strings.Contains(loc, "Moved+1+entry+to+a+waiting+list.") || !strings.Contains(loc, "1+entry+on+a+waiting+list") {
		t.Errorf("the organiser hears who's waiting: %s", loc)
	}
	// A limit that moves nobody is saved without asking.
	if loc := redirected(t, h, admin+"/limits", url.Values{"limit-0": {"2"}}); !strings.Contains(loc, "Limits+saved.") {
		t.Errorf("saved: %s", loc)
	}
	page := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(page, "2 of 2 places") || !strings.Contains(page, "Waiting lists") || !strings.Contains(page, `aria-label="Let in Cat Doyle"`) {
		t.Fatalf("BUCS L3 full, Cat waiting: %s", page)
	}
	if strings.Contains(page, `aria-label="Tick Cat Doyle"`) {
		t.Error("Cat isn't in the level's table")
	}
	if got := do(t, h, http.MethodGet, links[2], nil).Body.String(); !strings.Contains(got, "On the waiting list for BUCS L3: 1st") {
		t.Error("Cat sees she's first on the waiting list")
	}
	if strings.Contains(do(t, h, http.MethodGet, admin+"/entries.csv", nil).Body.String(), "Cat Doyle") {
		t.Error("waiting entries aren't exported")
	}

	// Let in over the limit, Cat can go back again from her entry's page.
	cat := regexp.MustCompile(`action="([^"]+)/let-in"`).FindStringSubmatch(page)[1]
	redirected(t, h, cat+"/let-in", url.Values{})
	if entry := do(t, h, http.MethodGet, cat, nil).Body.String(); !strings.Contains(entry, "Put it back on the waiting list") {
		t.Fatalf("a let-in entry can go back: %s", entry)
	}
	redirected(t, h, cat+"/let-in", url.Values{"on": {"0"}})
	if got := do(t, h, http.MethodGet, links[2], nil).Body.String(); !strings.Contains(got, "On the waiting list for BUCS L3: 1st") {
		t.Error("Cat is back on the waiting list, in her place")
	}

	// Clearing the limit would let Cat in: it asks, naming her.
	if body := do(t, h, http.MethodPost, admin+"/limits", url.Values{"limit-0": {""}}).Body.String(); !strings.Contains(body, "This lets 1 entry in from the waiting lists.") ||
		!strings.Contains(body, "Let in: Cat Doyle (On their own), BUCS L3") || !strings.Contains(body, "Save the limits") {
		t.Errorf("asks before letting Cat in: %s", body)
	}

	// Bea withdraws: Cat is in.
	if to := redirected(t, h, links[1]+"/withdraw", url.Values{"confirm": {"1"}}); !strings.Contains(noticeOf(t, to), "Withdrawn") {
		t.Fatal("Bea withdraws")
	}
	if got := do(t, h, http.MethodGet, links[2], nil).Body.String(); strings.Contains(got, "waiting list") {
		t.Error("Cat moves up when Bea withdraws")
	}
	if page := do(t, h, http.MethodGet, admin, nil).Body.String(); strings.Contains(page, "Waiting lists") || !strings.Contains(page, `aria-label="Tick Cat Doyle"`) {
		t.Error("Cat is in the table now")
	}
	if hist := do(t, h, http.MethodGet, admin+"/history", nil).Body.String(); !strings.Contains(hist, "Changed the limits") {
		t.Error("the history")
	}
}
