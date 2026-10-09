package web

import (
	"fmt"
	"net/http"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Synchro partners see the pair's entry (roadmap 2026-10-08): once the
// partner confirms, the entry the other gymnast made shows on the partner's
// own page, and to their coach and their club's comp sec, to see, as checked,
// with its sign-off and, once published, its flight. Both gymnasts' pages say
// when the pair is at the wrong level for their individual ones.

// sentEntries are a competition's entries and its pairs' level warnings, by
// entry id, read once a request.
type sentEntries struct {
	c     store.Competition
	byID  map[string]store.Entry
	warns map[string]string
}

// competitionsSeen reads competitions and their entries as a request needs
// them.
type competitionsSeen struct {
	p    *competitionPages
	r    *http.Request
	seen map[string]*sentEntries
}

func (p *competitionPages) seenFor(r *http.Request) *competitionsSeen {
	return &competitionsSeen{p: p, r: r, seen: map[string]*sentEntries{}}
}

// of is a competition, with its entries and their pairs' warnings.
func (cs *competitionsSeen) of(id string) (*sentEntries, error) {
	if s, ok := cs.seen[id]; ok {
		return s, nil
	}
	c, err := cs.p.st.Competition(cs.r.Context(), id)
	if err != nil {
		return nil, err
	}
	entries, err := cs.p.st.Entries(cs.r.Context(), id)
	if err != nil {
		return nil, err
	}
	s := &sentEntries{c: c, byID: map[string]store.Entry{}, warns: map[string]string{}}
	for _, e := range entries {
		s.byID[e.ID] = e
	}
	for i, w := range synchroLevels(c.Competition, entries) {
		s.warns[entries[i].ID] = w
	}
	cs.seen[id] = s
	return s, nil
}

// pairWarning is the wrong-level warning for a pair's entry, by the
// competition's copy of it ("" if there's none, or nothing to say).
func (cs *competitionsSeen) pairWarning(competitionID, copyID string) (string, error) {
	if copyID == "" {
		return "", nil
	}
	s, err := cs.of(competitionID)
	if err != nil {
		return "", err
	}
	return s.warns[copyID], nil
}

// pairViews are pair entries as the partner's side sees them, leaving out
// those skip says they see already.
func (cs *competitionsSeen) pairViews(pairs []store.PairEntry, skip func(store.PairEntry) bool) ([]views.PairView, error) {
	var out []views.PairView
	for _, pe := range pairs {
		if skip != nil && skip(pe) {
			continue
		}
		s, err := cs.of(pe.CompetitionID)
		if err != nil {
			return nil, err
		}
		entrant := pe.Entrant
		if pe.Club != "" {
			entrant += " (" + pe.Club + ")"
		}
		v := views.PairView{
			Competition: s.c.Name,
			Title:       fmt.Sprintf("%s · %s and %s", pe.Entry.Event(), pe.Entrant, pe.Entry.Partner.Name),
			EnteredBy:   entrant,
			Status:      "Not sent to the competition yet",
		}
		club := pe.Club
		if club == "" {
			club = "Individual"
		}
		shown, err := card(s.c.Competition, pe.Entry, club)
		if err != nil {
			return nil, err
		}
		signoff := views.SignoffView{Required: s.c.Signoff, Signed: !pe.SignedAt.IsZero(), By: pe.SignedBy}
		if copy, ok := s.byID[pe.CopyID]; ok {
			v.Status = "Sent to the competition"
			if copy.Checked() {
				v.Status += " · checked by the organiser ✓"
			}
			signoff = storedSignoff(s.c.Competition, copy)
			v.Placement = placement(s.c, copy.ID)
		}
		withSignoff(&shown, signoff)
		if w := s.warns[pe.CopyID]; w != "" {
			shown.Problems = append([]string{w}, shown.Problems...)
		}
		v.Card = shown
		out = append(out, v)
	}
	return out, nil
}
