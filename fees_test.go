package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestEntryFees(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	clubLink, enter := pathIn(t, dash, "/competitions/club/"), pathIn(t, dash, "/competitions/enter/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	join := pathIn(t, do(t, h, http.MethodGet, club, nil).Body.String(), "/clubs/join/")
	entry := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}
	for _, name := range []string{"Ann", "Bea"} {
		m := withoutQuery(redirected(t, h, join, url.Values{"name": {name}}))
		redirected(t, h, formAction(t, do(t, h, http.MethodGet, m, nil).Body.String(), m+"/competitions/"), entry)
	}
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, club, nil).Body.String(), club+"/competitions/"), url.Values{"which": {"all"}})
	ind := url.Values{"gymnast": {"Ivy"}}
	for k, v := range entry {
		ind[k] = v
	}
	ivy := withoutQuery(redirected(t, h, enter, ind))

	// Nothing charged: nothing said.
	if strings.Contains(do(t, h, http.MethodGet, club, nil).Body.String(), "Fees:") {
		t.Error("no fees, no fees line")
	}
	fees := admin + "/fees"
	if loc := redirected(t, h, fees+"/settings", url.Values{"fee-": {"abc"}}); !strings.Contains(loc, "Nothing+was+changed") {
		t.Errorf("a bad amount: %s", loc)
	}
	redirected(t, h, fees+"/settings", url.Values{"fee-": {"12"}, "club": {"25"}, "instructions": {"Bank transfer, IBAN IE00 TEST"}})

	page := do(t, h, http.MethodGet, fees, nil).Body.String()
	if !strings.Contains(page, "<td>UCD</td>") && !strings.Contains(page, "UCD") || !strings.Contains(page, "<td>€49</td>") || !strings.Contains(page, "<td>€12</td>") {
		t.Fatalf("UCD owes €49 (club €25 and two €12 entries), Ivy €12: %s", page)
	}

	// UCD pays €25: €24 to come.
	payer := "club:" + strings.Split(strings.Split(page, `name="payer" value="club:`)[1], `"`)[0]
	redirected(t, h, fees+"/payments", url.Values{"payer": {payer}, "amount": {"25"}, "note": {"Transfer"}})
	if page := do(t, h, http.MethodGet, club, nil).Body.String(); !strings.Contains(page, "<strong>€24 to pay</strong>") {
		t.Error("the comp sec sees what's left to pay")
	}
	clubPage := do(t, h, http.MethodGet, club, nil).Body.String()
	invoiceLink := club + "/competitions/" + strings.Split(strings.Split(clubPage, club+"/competitions/")[1], "/")[0] + "/invoice"
	invoice := do(t, h, http.MethodGet, invoiceLink, nil).Body.String()
	for _, want := range []string{"Club fee", "Ann · BUCS L3", "Bea · BUCS L3", "−€25", "IBAN IE00 TEST", "€24"} {
		if !strings.Contains(invoice, want) {
			t.Errorf("the club's invoice has %q", want)
		}
	}
	if !strings.Contains(do(t, h, http.MethodGet, ivy, nil).Body.String(), "€12 to pay") {
		t.Error("Ivy sees hers")
	}
	if inv := do(t, h, http.MethodGet, ivy+"/invoice", nil).Body.String(); !strings.Contains(inv, "Ivy · BUCS L3") || strings.Contains(inv, "Club fee") {
		t.Error("Ivy's invoice: her entry, no club fee")
	}

	// Only the organiser (and co-organisers) see the fees; the history says who recorded what.
	cards := madeLink(t, h, admin, "Judges", "cards")
	if code := do(t, h, http.MethodGet, cards+"/fees", nil).Code; code != http.StatusForbidden {
		t.Errorf("checking cards can't see fees: %d", code)
	}
	if hist := do(t, h, http.MethodGet, admin+"/history", nil).Body.String(); !strings.Contains(hist, "Recorded a payment of €25 from UCD") || !strings.Contains(hist, "Changed the fees") {
		t.Error("the history")
	}
}
