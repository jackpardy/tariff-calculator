package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Entry fees (roadmap 2026-10-08): the organiser sets what's charged and
// records payments; each club and individual sees what they owe, with an
// invoice to print.

func (p *competitionPages) registerFees(handle func(string, http.HandlerFunc)) {
	handle("GET /competitions/admin/{token}/fees", p.fees)
	handle("POST /competitions/admin/{token}/fees/settings", p.setFees)
	handle("POST /competitions/admin/{token}/fees/payments", p.addPayment)
	handle("POST /competitions/admin/{token}/fees/payments/{id}/remove", p.removePayment)
	handle("GET /competitions/admin/{token}/fees/invoice", p.adminInvoice)
	handle("GET /clubs/admin/{token}/competitions/{id}/invoice", p.clubInvoice)
	handle("GET /competitions/entry/{token}/invoice", p.individualInvoice)
}

// account is what one payer (a club, or an individual's entry) owes.
type account struct {
	key, name string
	club      bool
	entries   []store.Entry
	lines     []competitions.InvoiceLine
	due, paid int
	payments  []store.Payment
}

// clubPayer and entryPayer are payers' keys.
func clubPayer(clubID string) string   { return "club:" + clubID }
func entryPayer(entryID string) string { return "entry:" + entryID }

// accounts are every payer with entries (or payments): clubs as their
// entries came, then individuals, each entry its own.
func (p *competitionPages) accounts(ctx context.Context, c store.Competition) ([]account, error) {
	all, err := p.st.Entries(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	payments, err := p.st.Payments(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	var out []account
	index := map[string]int{}
	for _, e := range live(all) {
		key, name, club := entryPayer(e.ID), e.Entry.Gymnast+" (individual)", false
		if !e.Individual {
			key, name, club = clubPayer(e.ClubID), e.ClubName, true
		}
		i, ok := index[key]
		if !ok {
			i, index[key] = len(out), len(out)
			out = append(out, account{key: key, name: name, club: club})
		}
		out[i].entries = append(out[i].entries, e)
	}
	for _, pay := range payments {
		i, ok := index[pay.Payer]
		if !ok {
			continue // their entries have gone; the payment stays recorded
		}
		out[i].payments = append(out[i].payments, pay)
		out[i].paid += pay.Amount
	}
	for i := range out {
		var entries []competitions.Entry
		for _, e := range out[i].entries {
			entries = append(entries, e.Entry)
		}
		out[i].lines, out[i].due = c.Fees.Invoice(entries, out[i].club)
	}
	// Late changes accepted with a fee go on the payer's invoice.
	late, err := p.st.LateRequests(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	payerOf := map[string]string{}
	for _, e := range all {
		payerOf[e.ID] = entryPayer(e.ID)
		if !e.Individual {
			payerOf[e.ID] = clubPayer(e.ClubID)
		}
	}
	for _, r := range late {
		if r.Status != store.LateAccepted || r.Fee == 0 {
			continue
		}
		if i, ok := index[payerOf[r.EntryID]]; ok {
			out[i].lines = append(out[i].lines, competitions.InvoiceLine{What: competitions.LateKindName(r.Kind) + ": " + r.Entry.Gymnasts() + " · " + r.Entry.Event(), Amount: r.Fee})
			out[i].due += r.Fee
		}
	}
	return out, nil
}

// accountOf is one payer's account, if they have entries.
func (p *competitionPages) accountOf(ctx context.Context, c store.Competition, key string) (account, bool, error) {
	all, err := p.accounts(ctx, c)
	if err != nil {
		return account{}, false, err
	}
	for _, a := range all {
		if a.key == key {
			return a, true, nil
		}
	}
	return account{}, false, nil
}

// feesSummary is what a payer sees of their account, nil if the competition
// charges nothing or they've nothing entered.
func (p *competitionPages) feesSummary(ctx context.Context, c store.Competition, key, invoice string) *views.FeesSummary {
	if !c.Fees.On() {
		return nil
	}
	a, ok, err := p.accountOf(ctx, c, key)
	if err != nil || !ok {
		return nil
	}
	return &views.FeesSummary{Due: c.Fees.Money(a.due), Paid: c.Fees.Money(a.paid), Balance: c.Fees.Money(a.due - a.paid),
		Settled: a.paid >= a.due, Invoice: invoice}
}

// fees is the organiser's page: the fees, and each payer's account.
func (p *competitionPages) fees(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	accounts, err := p.accounts(r.Context(), c)
	if err != nil {
		failed(w, r, err)
		return
	}
	base := adminPath(r.PathValue("token"))
	f := c.Fees
	page := views.FeesPage{Base: base, Competition: summary(c.Competition, p.now()), Notice: r.URL.Query().Get("notice"), On: f.On(),
		GBP: f.Currency == competitions.CurrencyGBP, PerClub: amountField(f.PerClub), Instructions: f.Instructions}
	for _, d := range c.Disciplines() {
		page.PerEntry = append(page.PerEntry, views.FeeField{Key: d, Name: competitions.DisciplineName(d), Amount: amountField(f.PerEntry[d])})
	}
	due, paid := 0, 0
	for _, a := range accounts {
		v := views.AccountView{Payer: a.key, Name: a.name, Entries: len(a.entries), Due: f.Money(a.due), Paid: f.Money(a.paid),
			Balance: f.Money(a.due - a.paid), Settled: a.paid >= a.due, Invoice: base + "/fees/invoice?payer=" + url.QueryEscape(a.key)}
		for _, pay := range a.payments {
			v.Payments = append(v.Payments, views.PaymentView{ID: pay.ID, Amount: f.Money(pay.Amount), Note: pay.Note, Who: pay.Who,
				At: pay.At.In(local).Format("2 Jan")})
		}
		page.Accounts = append(page.Accounts, v)
		due, paid = due+a.due, paid+a.paid
	}
	page.Due, page.Paid, page.Balance = f.Money(due), f.Money(paid), f.Money(due-paid)
	render(w, r, views.FeesAdmin(page))
}

// amountField is an amount for a form: "" for nothing.
func amountField(cents int) string {
	if cents == 0 {
		return ""
	}
	return strings.ReplaceAll(strings.TrimPrefix(competitions.Fees{}.Money(cents), "€"), ",", "")
}

// setFees saves the fees: currency (GBP or not), fee-<discipline>, club,
// instructions.
func (p *competitionPages) setFees(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	f := competitions.Fees{PerEntry: map[string]int{}, Instructions: limitNote(strings.TrimSpace(r.FormValue("instructions")))}
	if r.FormValue("currency") == competitions.CurrencyGBP {
		f.Currency = competitions.CurrencyGBP
	}
	var problems []string
	for _, d := range c.Disciplines() {
		cents, err := competitions.ParseMoney(r.FormValue("fee-" + d))
		if err != nil {
			problems = append(problems, competitions.DisciplineName(d)+": "+err.Error()+".")
		}
		if cents > 0 {
			f.PerEntry[d] = cents
		}
	}
	cents, err := competitions.ParseMoney(r.FormValue("club"))
	if err != nil {
		problems = append(problems, "Club fee: "+err.Error()+".")
	}
	f.PerClub = cents
	back := adminPath(r.PathValue("token")) + "/fees"
	if len(problems) > 0 {
		http.Redirect(w, r, back+"?notice="+url.QueryEscape(strings.Join(problems, " ")+" Nothing was changed."), http.StatusSeeOther)
		return
	}
	if err := p.st.SetFees(r.Context(), c.ID, f); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, back+"?notice="+url.QueryEscape("Fees saved."), http.StatusSeeOther)
}

// addPayment records a payment (payer, amount, note).
func (p *competitionPages) addPayment(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	back := adminPath(r.PathValue("token")) + "/fees"
	cents, err := competitions.ParseMoney(r.FormValue("amount"))
	if err != nil || cents == 0 {
		http.Redirect(w, r, back+"?notice="+url.QueryEscape("Say how much was paid, like 12 or 12.50."), http.StatusSeeOther)
		return
	}
	if _, ok, err := p.accountOf(r.Context(), c, r.FormValue("payer")); err != nil || !ok {
		failed(w, r, errors.Join(err, store.ErrNotFound))
		return
	}
	if err := p.st.AddPayment(r.Context(), c.ID, r.FormValue("payer"), cents, limitNote(strings.TrimSpace(r.FormValue("note"))), linkOf(r).Name); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, back+"?notice="+url.QueryEscape(c.Fees.Money(cents)+" recorded."), http.StatusSeeOther)
}

// removePayment takes back a payment recorded by mistake.
func (p *competitionPages) removePayment(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := p.st.RemovePayment(r.Context(), c.ID, r.PathValue("id")); err != nil && !errors.Is(err, store.ErrNotFound) {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"/fees?notice="+url.QueryEscape("Payment removed."), http.StatusSeeOther)
}

// adminInvoice is a payer's invoice, for the organiser.
func (p *competitionPages) adminInvoice(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	p.invoice(w, r, c, r.URL.Query().Get("payer"), adminPath(r.PathValue("token"))+"/fees")
}

// clubInvoice is the club's invoice, for its comp sec.
func (p *competitionPages) clubInvoice(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	c, ok := p.enteredCompetition(w, r, club.ID)
	if !ok {
		return
	}
	p.invoice(w, r, c, clubPayer(club.ID), clubPath(r.PathValue("token")))
}

// individualInvoice is an individual's invoice for their entry.
func (p *competitionPages) individualInvoice(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.own(w, r)
	if !ok {
		return
	}
	p.invoice(w, r, c, entryPayer(e.ID), "/competitions/entry/"+r.PathValue("token"))
}

// invoice shows a payer's invoice, to print.
func (p *competitionPages) invoice(w http.ResponseWriter, r *http.Request, c store.Competition, key, back string) {
	a, ok, err := p.accountOf(r.Context(), c, key)
	if err != nil {
		failed(w, r, err)
		return
	}
	if !ok || !c.Fees.On() {
		message(w, r, http.StatusNotFound, "No invoice", "There's nothing to pay for this competition yet.")
		return
	}
	f := c.Fees
	page := views.InvoicePage{Competition: summary(c.Competition, p.now()), Back: back, To: a.name, Instructions: f.Instructions,
		Total: f.Money(a.due), Paid: f.Money(a.paid), Balance: f.Money(a.due - a.paid), Settled: a.paid >= a.due, Date: p.now().In(local).Format("2 January 2006")}
	for _, l := range a.lines {
		page.Lines = append(page.Lines, views.InvoiceLineView{What: l.What, Amount: f.Money(l.Amount)})
	}
	for _, pay := range a.payments {
		page.Payments = append(page.Payments, views.PaymentView{Amount: f.Money(pay.Amount), Note: pay.Note, At: pay.At.In(local).Format("2 January")})
	}
	render(w, r, views.Invoice(page))
}
