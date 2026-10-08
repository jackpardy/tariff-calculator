package main

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
	entries, err := p.st.Entries(ctx, competitionID)
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
			backToDashboard(w, r, fmt.Sprintf("%s: a limit should be a number of entries, or empty for none. Nothing was changed.", event))
			return
		}
		if n > 0 {
			limits[event] = n
		}
	}
	p.notify.changing(r.Context(), c)
	if err := p.st.SetLimits(r.Context(), c.ID, limits); err != nil {
		failed(w, r, err)
		return
	}
	entries, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	_, waiting := splitWaiting(entries)
	notice := "Limits saved."
	if len(waiting) > 0 {
		notice += fmt.Sprintf(" %s on a waiting list.", entriesWord(len(waiting)))
	}
	backToDashboard(w, r, notice)
}

// letIn lets an entry in over its event's limit (on=1), or takes that back.
func (p *competitionPages) letIn(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	on := r.FormValue("on") != "0"
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
