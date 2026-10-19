package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Limits per event, with a waiting list (roadmap 2026-10-08): once an event
// is full, later entries wait in the order they came in, and move up as
// places free; the organiser can let one in over the limit.

func (p *competitionPages) registerLimits(handle func(string, http.HandlerFunc)) {
	handle("POST /competitions/admin/{token}/limits", p.setLimits)
	handle("POST /competitions/admin/{token}/entries/{id}/let-in", p.letIn)
}

// splitWaiting are the entries in, and those waiting.
func splitWaiting(entries []store.Entry) (in, waiting []store.Entry) {
	for _, e := range entries {
		if e.Waiting > 0 {
			waiting = append(waiting, e)
		} else {
			in = append(in, e)
		}
	}
	return in, waiting
}

// waitingLists are each event's waiting list, in the competition's order of
// events, each first to last.
func waitingLists(c store.Competition, waiting []store.Entry, base string) []views.WaitingList {
	byEvent := map[string][]store.Entry{}
	for _, e := range waiting {
		byEvent[e.Entry.Event()] = append(byEvent[e.Entry.Event()], e)
	}
	var out []views.WaitingList
	for _, event := range c.EventNames() {
		list := byEvent[event]
		if len(list) == 0 {
			continue
		}
		wl := views.WaitingList{Event: event, Limit: c.Limits[event]}
		for place := 1; place <= len(list); place++ {
			for _, e := range list {
				if e.Waiting == place {
					wl.Rows = append(wl.Rows, views.WaitingRow{ID: e.ID, Place: place, Gymnast: e.Entry.Gymnasts(), Club: clubOf(e),
						Entered: e.EnteredAt.In(local).Format("2 Jan, 15:04"), Link: base + "/entries/" + e.ID})
				}
			}
		}
		out = append(out, wl)
	}
	return out
}

// limitFields are each event's limit, for the form, with how many have
// entered.
func limitFields(c store.Competition, entries []store.Entry) []views.LimitField {
	entered := map[string]int{}
	for _, e := range entries {
		if !e.Withdrawn {
			entered[e.Entry.Event()]++
		}
	}
	var out []views.LimitField
	for _, event := range c.EventNames() {
		f := views.LimitField{Event: event, Entered: entered[event]}
		if limit := c.Limits[event]; limit > 0 {
			f.Limit = strconv.Itoa(limit)
		}
		out = append(out, f)
	}
	return out
}

// waitingWord says where an entry is on its waiting list, e.g. "3rd".
func waitingWord(place int) string {
	suffix := "th"
	if place%100 < 11 || place%100 > 13 {
		switch place % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(place) + suffix
}

// waitingText says an entry is on its event's waiting list, "" if it's in.
func waitingText(e store.Entry) string {
	if e.Waiting == 0 {
		return ""
	}
	return fmt.Sprintf("On the waiting list for %s: %s. It moves up when a place frees; the organiser can also let it in.", e.Entry.Event(), waitingWord(e.Waiting))
}

// waitingOf is an entry's place on its waiting list (0: in), by its id.
func (p *competitionPages) waitingOf(ctx context.Context, competitionID, id string) store.Entry {
	entries, err := p.entriesOf(ctx, competitionID)
	if err != nil {
		return store.Entry{}
	}
	for _, e := range entries {
		if e.ID == id {
			return e
		}
	}
	return store.Entry{}
}

// setLimits saves each event's limit (limit-<n>, by the events' order;
// empty or 0: none).
func (p *competitionPages) setLimits(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	limits := map[string]int{}
	for i, event := range c.EventNames() {
		v := strings.TrimSpace(r.FormValue("limit-" + strconv.Itoa(i)))
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > store.MaxEntries {
			backToSettings(w, r, fmt.Sprintf("%s: a limit should be a number of entries, or empty for none. Nothing was changed.", event))
			return
		}
		if n > 0 {
			limits[event] = n
		}
	}
	// What the new limits do: entries let in from the waiting lists, and
	// entries in now put on them. Either asks first, saying who.
	entries, err := p.entriesOf(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	in, out := limitMoves(entries, store.WithLimits(limits, entries))
	if (len(in) > 0 || len(out) > 0) && !asked(r) {
		page := views.AskFirstPage{What: "Change the limits", Says: limitMovesText(len(in), len(out), false), Do: "Save the limits",
			Back: settingsPath(r.PathValue("token")) + "#limits"}
		for _, e := range in {
			page.Items = append(page.Items, "Let in: "+limitMoveName(e))
		}
		for _, e := range out {
			page.Items = append(page.Items, "To the waiting list: "+limitMoveName(e))
		}
		p.askBefore(w, r, c, menuSettings, page)
		return
	}
	p.notify.changing(r.Context(), c)
	if err := p.st.SetLimits(r.Context(), c.ID, limits); err != nil {
		failed(w, r, err)
		return
	}
	if entries, err = p.entriesOf(r.Context(), c.ID); err != nil {
		failed(w, r, err)
		return
	}
	_, waiting := splitWaiting(entries)
	notice := "Limits saved."
	if len(in) > 0 || len(out) > 0 {
		notice += " " + limitMovesText(len(in), len(out), true)
	}
	if len(waiting) > 0 {
		notice += fmt.Sprintf(" %s on a waiting list.", views.Count(len(waiting), "entry", "entries"))
	}
	backToSettings(w, r, notice)
}

// limitMoves are the entries new limits let in from the waiting lists (in),
// and those they put on them (out): entries before and after the change, in
// the same order.
func limitMoves(before, after []store.Entry) (in, out []store.Entry) {
	for i, e := range before {
		switch {
		case e.Waiting > 0 && after[i].Waiting == 0:
			in = append(in, e)
		case e.Waiting == 0 && after[i].Waiting > 0:
			out = append(out, e)
		}
	}
	return in, out
}

// limitMovesText says what new limits do, e.g. "This lets 14 entries in from
// the waiting lists.", or once done what they did ("Let 14 entries in…").
func limitMovesText(in, out int, done bool) string {
	let := fmt.Sprintf("lets %s in from the waiting lists", views.Count(in, "entry", "entries"))
	put := fmt.Sprintf("moves %s in now to a waiting list", views.Count(out, "entry", "entries"))
	if done {
		let = fmt.Sprintf("let %s in from the waiting lists", views.Count(in, "entry", "entries"))
		put = fmt.Sprintf("moved %s to a waiting list", views.Count(out, "entry", "entries"))
	}
	var says string
	switch {
	case in > 0 && out > 0:
		says = let + ", and " + put
	case in > 0:
		says = let
	default:
		says = put
	}
	if done {
		return strings.ToUpper(says[:1]) + says[1:] + "."
	}
	return "This " + says + "."
}

// limitMoveName is an entry as the limits' question lists it: "Ann Ryan
// (UCD), BUCS L3".
func limitMoveName(e store.Entry) string {
	return e.Entry.Gymnasts() + " (" + clubOf(e) + "), " + e.Entry.Event()
}

// letIn lets an entry in over its event's limit (on=1), or takes that back.
func (p *competitionPages) letIn(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	on := r.FormValue("on") != "0"
	e, err := p.st.CompetitionEntry(r.Context(), c.ID, r.PathValue("id"))
	if err != nil {
		failed(w, r, err)
		return
	}
	if e.LetIn == on { // let in already, or never was: nothing to do (nor to record)
		backToDashboard(w, r, "It's like that already; nothing was changed.")
		return
	}
	p.notify.changing(r.Context(), c)
	if err := p.st.LetIn(r.Context(), c.ID, r.PathValue("id"), on); err != nil {
		failed(w, r, err)
		return
	}
	notice := "Let in from the waiting list."
	if !on {
		notice = "Back to the waiting list, in its place."
	}
	backToDashboard(w, r, notice)
}
