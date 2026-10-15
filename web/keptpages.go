package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"tariffCalculator/requirements"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Requirements kept with the person (ADR 0013): a person's own requirement
// sets and levels, on their page whichever personal link they open it by
// (a member's, coach's or individual's: ADR 0010's one person). The
// Requirements page and the Routine Builder read them as JSON and keep a
// copy in the browser (KeptRequirements in static/js/sets.js); the person's
// Requirements tab lists them, adds those saved in this browser, and takes
// them off.

// personalBases are the personal links' pages, by the path before the token.
var personalBases = []string{"/clubs/member/", "/clubs/coach/", "/competitions/gymnast/"}

func (p *competitionPages) registerKept(handle func(string, http.HandlerFunc)) {
	handle("GET /clubs/member/{token}/requirements", p.memberKept)
	handle("GET /clubs/coach/{token}/requirements", p.coachKept)
	handle("GET /competitions/gymnast/{token}/requirements", p.gymnastKept)
	for _, base := range personalBases {
		handle("GET "+base+"{token}/requirements.json", p.keptJSON)
		handle("POST "+base+"{token}/requirements", p.keepRequirements)
		handle("POST "+base+"{token}/requirements/{kind}/{source}/remove", p.dropKept)
	}
}

// keptPath is the person's Requirements tab, under the page a request came
// by (its path up to the token).
func keptPath(r *http.Request) string {
	for _, base := range personalBases {
		if strings.HasPrefix(r.URL.Path, base) {
			return base + r.PathValue("token") + "/requirements"
		}
	}
	return "/"
}

// keptPerson is the person a personal link belongs to.
func (p *competitionPages) keptPerson(w http.ResponseWriter, r *http.Request) (string, bool) {
	h, err := p.st.HolderOf(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return "", false
	}
	return h.PersonID, true
}

// keptItem is one of a person's kept requirements, as the browser has it:
// its id there, its kind, its JSON (and for a set, its rules in words, as
// the Requirements page lists them).
type keptItem struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Data      json.RawMessage `json:"data"`
	Described []string        `json:"described,omitempty"`
}

// keptJSON is the person's kept requirements, for the browser:
// {"sets": [...], "levels": [...]}, each a keptItem.
func (p *competitionPages) keptJSON(w http.ResponseWriter, r *http.Request) {
	person, ok := p.keptPerson(w, r)
	if !ok {
		return
	}
	kept, err := p.st.KeptRequirements(r.Context(), person)
	if err != nil {
		failed(w, r, err)
		return
	}
	out := map[string][]keptItem{"sets": {}, "levels": {}}
	for _, k := range kept {
		item := keptItem{ID: k.Source, Kind: k.Kind, Data: json.RawMessage(k.Data)}
		if k.Kind == store.KeptSet {
			item.Described = describedSet(k.Data)
			out["sets"] = append(out["sets"], item)
		} else {
			out["levels"] = append(out["levels"], item)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(out)
}

// describedSet is a kept set's rules in words; nil where they can't be read.
func describedSet(data string) []string {
	var set requirements.Set
	if json.Unmarshal([]byte(data), &set) != nil {
		return nil
	}
	var out []string
	for _, rule := range set.Rules {
		out = append(out, describeKeptRule(rule))
	}
	return out
}

// describeKeptRule is a rule in words, as far as it can be: a set kept with
// problems may have a rule its describer can't read.
func describeKeptRule(rule requirements.Rule) (text string) {
	defer func() {
		if recover() != nil {
			text = "A rule that needs fixing"
		}
	}()
	return requirements.Describe(rule)
}

// keepRequirements keeps the posted requirements on the person's page:
// items, a JSON list of keptItem (a set's or level's id in the browser, its
// kind, its JSON, which must have a name). With back=1 (the person's own
// Requirements tab, adding those in this browser) it goes back there with
// what happened; otherwise it answers in JSON, {"kept": n, "refused": [...]},
// for the Requirements page's Save to my page.
func (p *competitionPages) keepRequirements(w http.ResponseWriter, r *http.Request) {
	person, ok := p.keptPerson(w, r)
	if !ok {
		return
	}
	var items []keptItem
	if err := json.Unmarshal([]byte(r.FormValue("items")), &items); err != nil {
		badRequest(w, fmt.Errorf("those requirements couldn't be read: %w", err))
		return
	}
	kept, added := 0, 0
	var refused []string
	for _, it := range items {
		var named struct{ Name string }
		if json.Unmarshal(it.Data, &named) != nil || strings.TrimSpace(named.Name) == "" {
			refused = append(refused, "one without a name")
			continue
		}
		isNew, err := p.st.KeepRequirements(r.Context(), person, it.Kind, it.ID, named.Name, string(it.Data))
		switch {
		case errors.Is(err, store.ErrLimit):
			refused = append(refused, fmt.Sprintf("%s (you keep %d already, the most there's room for)", named.Name, store.MaxKept))
		case errors.Is(err, store.ErrTooBig):
			refused = append(refused, named.Name+" (too big)")
		case err != nil && strings.Contains(err.Error(), "not something to keep"):
			refused = append(refused, named.Name)
		case err != nil:
			failed(w, r, err)
			return
		default:
			kept++
			if isNew {
				added++
			}
		}
	}
	if r.FormValue("back") != "" {
		notice := fmt.Sprintf("Kept %d on your page.", kept)
		switch kept {
		case 0:
			notice = "Nothing was kept."
		case 1:
			notice = "Kept it on your page."
		}
		if len(refused) > 0 {
			notice += " Not kept: " + strings.Join(refused, "; ") + "."
		}
		seeOther(w, r, keptPath(r), notice)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"kept": kept, "added": added, "refused": refused})
}

// dropKept takes one of the person's kept requirements off their page, by
// its kind and its id in the browser; back=1 goes back to their
// Requirements tab, otherwise it answers with no content.
func (p *competitionPages) dropKept(w http.ResponseWriter, r *http.Request) {
	person, ok := p.keptPerson(w, r)
	if !ok {
		return
	}
	if err := p.st.DropKept(r.Context(), person, r.PathValue("kind"), r.PathValue("source")); err != nil {
		failed(w, r, err)
		return
	}
	if r.FormValue("back") != "" {
		seeOther(w, r, keptPath(r), "Taken off your page.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// keptPart is the person's kept requirements as their Requirements tab
// shows them.
func (p *competitionPages) keptPart(w http.ResponseWriter, r *http.Request) (views.KeptPart, bool) {
	person, ok := p.keptPerson(w, r)
	if !ok {
		return views.KeptPart{}, false
	}
	kept, err := p.st.KeptRequirements(r.Context(), person)
	if err != nil {
		failed(w, r, err)
		return views.KeptPart{}, false
	}
	base := keptPath(r)
	part := views.KeptPart{Add: base, Max: store.MaxKept}
	names := map[string]string{} // kept sets' names, by their id in the browser, for levels to name them
	for _, k := range kept {
		if k.Kind == store.KeptSet {
			names[k.Source] = k.Name
		}
	}
	for _, k := range kept {
		v := views.KeptView{Name: k.Name, Updated: k.Updated.In(local).Format("2 January 2006"), Remove: base + "/" + k.Kind + "/" + k.Source + "/remove"}
		if k.Kind == store.KeptSet {
			v.Lines = describedSet(k.Data)
			part.Sets = append(part.Sets, v)
			continue
		}
		v.Lines = levelLines(k.Data, names)
		part.Levels = append(part.Levels, v)
	}
	return part, true
}

// levelLines say what a kept level's exercises are checked against: each
// option a built-in's name, a kept set's, or "requirements not kept here".
func levelLines(data string, names map[string]string) []string {
	var level requirements.Level
	if json.Unmarshal([]byte(data), &level) != nil {
		return nil
	}
	name := func(ref string) string {
		if set, ok := requirements.LookupBuiltin(ref); ok {
			return set.Name
		}
		if n, ok := names[ref]; ok {
			return n
		}
		return "requirements not kept here"
	}
	options := func(ex *requirements.Exercise) string {
		if ex == nil {
			return ""
		}
		var out []string
		for _, ref := range ex.Options {
			out = append(out, name(ref))
		}
		return strings.Join(out, " or ")
	}
	if level.Second == nil {
		return []string{"Both exercises: " + options(&level.First)}
	}
	return []string{"First exercise: " + options(&level.First), "Second exercise: " + options(level.Second)}
}

// memberKept is a member's Requirements tab.
func (p *competitionPages) memberKept(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	page, _, ok := p.memberPage(w, r, m, memberRequirementsTab)
	if !ok {
		return
	}
	if page.Kept, ok = p.keptPart(w, r); ok {
		render(w, r, views.MemberRequirements(page))
	}
}

// coachKept is a coach's Requirements tab.
func (p *competitionPages) coachKept(w http.ResponseWriter, r *http.Request) {
	coach, club, ok := p.coach(w, r)
	if !ok {
		return
	}
	if part, ok := p.keptPart(w, r); ok {
		render(w, r, views.CoachRequirements(coachFrame(r, coach, club, coachRequirementsTab), part))
	}
}

// gymnastKept is an individual's Requirements tab.
func (p *competitionPages) gymnastKept(w http.ResponseWriter, r *http.Request) {
	ind, _, ok := p.gymnast(w, r)
	if !ok {
		return
	}
	page := p.gymnastPage(r, ind, gymnastRequirements)
	if page.Kept, ok = p.keptPart(w, r); ok {
		render(w, r, views.GymnastRequirements(page))
	}
}
