package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// The comp sec enters a member from the competition's Entries: who, and
// in what, opening the member's entry form.
func TestEnterAMember(t *testing.T) {
	h := competitionServer(t)
	club := aClub(t, h, created(t, h, newCompetition()), "UCD", false, "A. Murphy")
	page := clubEntriesPage(t, h, club.page)
	action := regexp.MustCompile(`action="([^"]+/enter-member)"`).FindStringSubmatch(page)
	member := regexp.MustCompile(`<option value="([^"]+)">A. Murphy</option>`).FindStringSubmatch(page)
	if action == nil || member == nil {
		t.Fatalf("Enter a member, with the club's members: %s", page)
	}
	if to := do(t, h, http.MethodGet, action[1]+"?member="+member[1]+"&discipline=", nil).Header().Get("Location"); !strings.Contains(to, "/members/"+member[1]+"/competitions/") {
		t.Errorf("to the member's entry form: %s", to)
	}
	if to := do(t, h, http.MethodGet, action[1]+"?member=nobody", nil).Header().Get("Location"); !strings.Contains(to, "Choose+a+member") {
		t.Errorf("someone not a member: %s", to)
	}
}

// The comp sec's entry form for a member: the discipline named, a synchro
// partner chosen from the club's members, and Withdraw once entered.
func TestCompSecEntersAndWithdraws(t *testing.T) {
	h := competitionServer(t)
	club := aClub(t, h, created(t, h, newCompetition()), "UCD", false, "A. Murphy", "B. Byrne")
	page := clubEntriesPage(t, h, club.page)
	action := regexp.MustCompile(`action="([^"]+/enter-member)"`).FindStringSubmatch(page)[1]
	member := regexp.MustCompile(`<option value="([^"]+)">A. Murphy</option>`).FindStringSubmatch(page)[1]

	synchro := do(t, h, http.MethodGet, do(t, h, http.MethodGet, action+"?member="+member+"&discipline=synchro", nil).Header().Get("Location"), nil).Body.String()
	if !strings.Contains(synchro, "Enter them in synchro") || !strings.Contains(synchro, `<option value="B. Byrne">`) || strings.Contains(synchro, `<option value="A. Murphy">`) {
		t.Errorf("synchro, with the other members to choose: %s", synchro)
	}

	form := do(t, h, http.MethodGet, action+"?member="+member+"&discipline=", nil).Header().Get("Location")
	form = strings.SplitN(form, "?", 2)[0]
	if !strings.Contains(do(t, h, http.MethodGet, form, nil).Body.String(), "Enter them in trampoline") {
		t.Error("trampoline named")
	}
	redirected(t, h, form, bucsEntry(""))
	entered := do(t, h, http.MethodGet, form, nil).Body.String()
	if !strings.Contains(entered, `action="`+form+`/withdraw"`) || strings.Index(entered, "Change the entry") > strings.Index(entered, "comp-exercise") {
		t.Errorf("Change above the card, Withdraw below: %s", entered)
	}
	if to := redirected(t, h, form+"/withdraw", url.Values{"confirm": {"1"}, "discipline": {""}}); !strings.Contains(noticeOf(t, to), "Withdrew A. Murphy's entry") {
		t.Errorf("withdrawn: %s", to)
	}
	if !strings.Contains(do(t, h, http.MethodGet, form, nil).Body.String(), "Enter them in trampoline") {
		t.Error("not entered any more")
	}
}
