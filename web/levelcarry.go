package web

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Carrying judges' levels over a rename (ADR 0017, 2026-11-13). A level
// renamed on the events settings leaves the offers that said "up to" its old
// name flagged. The organiser is asked, once the change is saved, whether to
// carry them to the new name; nothing is carried unless they say. A rename
// they leave is kept with the competition's officials settings, so the
// Officials page can still offer it.

// carryKey is a rename as the form posts it: its discipline and old name.
func carryKey(r competitions.LevelRename) string { return r.Discipline + ":" + r.From }

// noteRenames keeps the levels the change renamed, to carry later. It says
// whether any offer names one.
func (p *competitionPages) noteRenames(ctx context.Context, c store.Competition, changed competitions.Competition) (bool, error) {
	settings := c.Officials
	for _, d := range []string{competitions.Tumbling, competitions.DMT} {
		settings.NoteRenames(d, competitions.RenamedLevels(c.LevelNames(d), changed.LevelNames(d)))
	}
	if slices.Equal(settings.Renamed, c.Officials.Renamed) {
		return false, nil
	}
	changed.Officials = settings
	pending, err := p.pendingRenames(ctx, c.ID, changed)
	if err != nil {
		return false, err
	}
	settings.Renamed = nil
	for _, r := range pending {
		settings.Renamed = append(settings.Renamed, r.rename)
	}
	if err := p.st.SetOfficialSettings(ctx, c.ID, settings); err != nil {
		return false, err
	}
	return len(pending) > 0 && !slices.Equal(settings.Renamed, c.Officials.Renamed), nil
}

// pendingRename is a rename that can still be carried, with who it would
// carry.
type pendingRename struct {
	rename competitions.LevelRename
	judges int
}

// pendingRenames are the competition's renames still to carry: the new name
// is a level, the old isn't, and some offer says up to the old one.
func (p *competitionPages) pendingRenames(ctx context.Context, id string, c competitions.Competition) ([]pendingRename, error) {
	var out []pendingRename
	for _, r := range c.Officials.Renamed {
		if !slices.Contains(c.LevelNames(r.Discipline), r.To) || slices.Contains(c.LevelNames(r.Discipline), r.From) {
			continue
		}
		n, err := p.st.LevelOffers(ctx, id, r.Discipline, r.From)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			out = append(out, pendingRename{r, n})
		}
	}
	return out, nil
}

// carryAsk is the ask to carry the competition's pending renames over, or nil
// for none. leave is where Leave them goes ("" for no button).
func (p *competitionPages) carryAsk(ctx context.Context, r *http.Request, c store.Competition, back, leave string) (*views.CarryAsk, error) {
	if !can(r, "POST", "/events/carry") {
		return nil, nil
	}
	pending, err := p.pendingRenames(ctx, c.ID, c.Competition)
	if err != nil || len(pending) == 0 {
		return nil, err
	}
	ask := &views.CarryAsk{Action: adminPath(r.PathValue("token")) + "/events/carry", Back: leave, Return: back}
	for _, k := range pending {
		ask.Renames = append(ask.Renames, views.CarryRename{Key: carryKey(k.rename), Discipline: competitions.DisciplineName(k.rename.Discipline),
			From: k.rename.From, To: k.rename.To, Judges: k.judges})
	}
	return ask, nil
}

// carryLevels carries the ticked renames (carry, each a discipline and old
// name) over in every offer of the competition, and writes each in the
// history.
func (p *competitionPages) carryLevels(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	back := settingsPath(r.PathValue("token")) + "#events"
	if r.FormValue("back") == "officials" {
		back = officialsPath(r)
	}
	pending, err := p.pendingRenames(r.Context(), c.ID, c.Competition)
	if err != nil {
		failed(w, r, err)
		return
	}
	var chosen []competitions.LevelRename
	for _, k := range pending {
		if slices.Contains(r.Form["carry"], carryKey(k.rename)) {
			chosen = append(chosen, k.rename)
		}
	}
	if len(chosen) == 0 {
		refuse(w, r, back, "Nothing was carried: no renamed level was chosen, or it's carried already.")
		return
	}
	counts, err := p.st.CarryLevels(r.Context(), c.ID, chosen)
	if err != nil {
		failed(w, r, err)
		return
	}
	who := linkOf(r).Name
	var done []string
	for i, k := range chosen {
		what := fmt.Sprintf("Carried %s from %s to %s", judgesLevels(counts[i]), k.From, k.To)
		p.st.Record(context.WithoutCancel(r.Context()), c.ID, who, what)
		done = append(done, what+".")
	}
	seeOther(w, r, back, strings.Join(done, " "))
}

// judgesLevels says whose levels were carried, e.g. "7 judges' levels".
func judgesLevels(n int) string {
	if n == 1 {
		return "1 judge's level"
	}
	return fmt.Sprintf("%d judges' levels", n)
}
