package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"tariffCalculator/requirements"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The level editor on the club's page (ADR 0011 Decision 5, roadmap's
// requirements workflow step 3): a coach writes one of the club's levels in
// the requirements editor, with the kinds of rule a level tracks and the
// club's own named skills to pick, and saves it straight to the club. One
// copy, on any phone; the Requirements page's Save to club levels stays as
// another way in.

// clubPicks are the club's own named skills, for the editor to offer first:
// each as its skill, in its shape where the shape counts, named as the club
// names it.
func (p *competitionPages) clubPicks(r *http.Request, clubID string) ([]requirements.Pick, error) {
	named, _, err := p.clubSkillsOf(r, clubID)
	if err != nil {
		return nil, err
	}
	var out []requirements.Pick
	for _, s := range named {
		if s.CustomName == "" {
			continue
		}
		m := requirements.SkillMatch(s)
		if s.ShapeIsRelevant() {
			m.Shapes = []string{strings.ToLower(s.Shape.String())}
		}
		m.Label = s.CustomName
		out = append(out, requirements.Pick{ID: "club:" + s.CustomName, Label: s.CustomName, Group: "Your club's skills", Match: m})
	}
	slices.SortFunc(out, func(a, b requirements.Pick) int { return strings.Compare(a.Label, b.Label) })
	return out, nil
}

// clubOwn are the club's own skills to track (not trampoline skills), for a
// level's rule of one of them, by name.
func (p *competitionPages) clubOwn(r *http.Request, clubID string) ([]views.ClubThing, error) {
	_, other, err := p.clubSkillsOf(r, clubID)
	if err != nil {
		return nil, err
	}
	out := make([]views.ClubThing, 0, len(other))
	for _, c := range other {
		out = append(out, views.ClubThing{ID: c.ID, Name: c.Name})
	}
	slices.SortFunc(out, func(a, b views.ClubThing) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// pickedRule is the rule for a skill picked from Add a skill (pick): one of
// the club's own skills to track ("own:<id>"), or a skill or kind of skill
// the gymnast can do (a pick's ID, the club's named skills among picks).
func pickedRule(pick string, picks []requirements.Pick) (requirements.Rule, bool) {
	if pick == "" {
		return requirements.Rule{}, false
	}
	if id, own := strings.CutPrefix(pick, "own:"); own {
		return requirements.Rule{Type: requirements.Track, Item: id}, id != ""
	}
	p, ok := requirements.PickByID(pick)
	if i := slices.IndexFunc(picks, func(p requirements.Pick) bool { return p.ID == pick }); i >= 0 {
		p, ok = picks[i], true
	}
	if !ok {
		return requirements.Rule{}, false
	}
	rule := newRule("include")
	m := p.Match.Clone()
	if p.Group == "Your club's skills" {
		m.Label = p.Label
	}
	rule.Match = &m
	return rule, true
}

// levelEditorData is the editor as it shows a level being written: the
// set, its problems and the rules that can't be met together, and where it
// posts on the coach's page (token) for the level (id, "" for a new one).
func levelEditorData(set requirements.Set, problems []string, token, id string, picks []requirements.Pick, own []views.ClubThing) views.SetEditorData {
	if set.Rules == nil {
		set.Rules = []requirements.Rule{}
	}
	// One of the club's own skills just added is the first, until chosen.
	for i := range set.Rules {
		if set.Rules[i].Type == requirements.Track && set.Rules[i].Item == "" && len(own) > 0 {
			set.Rules[i].Item = own[0].ID
		}
	}
	var conflicts []string
	if err := set.Validate(); err != nil {
		problems = append(problems, strings.Split(err.Error(), "\n")...)
	} else {
		conflicts = requirements.Conflicts(set)
	}
	// A rule fitting any skill is met by anything: say which skill.
	problems = append(problems, requirements.Vague(set)...)
	data, _ := json.Marshal(set)
	save := coachLevelsPath(token) + "/new"
	if id != "" {
		save = coachLevelsPath(token) + "/" + id + "/edit"
	}
	return views.SetEditorData{Set: set, JSON: string(data), Problems: problems, Conflicts: conflicts, OpenRule: -1, Club: &views.ClubEditor{
		Endpoint: coachLevelsPath(token) + "/editor?level=" + id, Save: save, Back: coachLevelsPath(token), Picks: picks, Own: own,
	}}
}

// newClubLevel is the editor for a new level: blank, or a copy of one of the
// club's levels (from).
func (p *competitionPages) newClubLevel(w http.ResponseWriter, r *http.Request) {
	coach, club, ok := p.coach(w, r)
	if !ok {
		return
	}
	set := requirements.Set{Format: requirements.Format, Name: "New level", Rules: []requirements.Rule{}}
	// A competition's set routine to start from: the level's first item.
	if ref := r.URL.Query().Get("set"); ref != "" {
		if builtin, ok := requirements.LookupBuiltin(ref); ok {
			if _, isSet := requirements.SetRoutine(builtin); isSet {
				i := slices.IndexFunc(builtin.Rules, func(rule requirements.Rule) bool { return rule.Type == requirements.Sequence })
				rule := builtin.Rules[i]
				if rule.Label == "" {
					rule.Label = builtin.Name
				}
				set.Name, set.Rules = builtin.Name, []requirements.Rule{rule}
			}
		}
	}
	if from := r.URL.Query().Get("from"); from != "" {
		level, found, err := p.clubLevel(r, coach, from)
		if err != nil {
			failed(w, r, err)
			return
		}
		if found {
			if copied, err := requirements.Parse([]byte(level.Set)); err == nil {
				set, set.Name = copied, "Copy of "+level.Name
				set.Rules = withClubItems(set.Rules, level.ClubItems)
			}
		}
	}
	p.showLevelEditor(w, r, coach, club, "New level", set, nil, "")
}

// editClubLevel is the editor for one of the club's levels.
func (p *competitionPages) editClubLevel(w http.ResponseWriter, r *http.Request) {
	coach, club, ok := p.coach(w, r)
	if !ok {
		return
	}
	level, found, err := p.clubLevel(r, coach, r.PathValue("id"))
	switch {
	case err != nil:
		failed(w, r, err)
		return
	case !found:
		http.NotFound(w, r)
		return
	}
	set, err := requirements.Parse([]byte(level.Set))
	if err != nil {
		badRequest(w, fmt.Errorf("that level's requirements couldn't be read: %w", err))
		return
	}
	// The club skills it lists after its rules (from before they could be
	// rules) open as rules at the end, to move where they belong.
	set.Rules = withClubItems(set.Rules, level.ClubItems)
	p.showLevelEditor(w, r, coach, club, level.Name, set, nil, level.ID)
}

// showLevelEditor shows the editor page for a level (id, "" for a new one).
func (p *competitionPages) showLevelEditor(w http.ResponseWriter, r *http.Request, coach store.Coach, club, title string, set requirements.Set, problems []string, id string) {
	picks, err := p.clubPicks(r, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	own, err := p.clubOwn(r, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	render(w, r, views.ClubLevelEditor(views.LevelEditorPage{
		Frame: coachFrame(r, coach, club, coachLevelsTab), Title: title,
		Editor: levelEditorData(set, problems, r.PathValue("token"), id, picks, own),
	}))
}

// clubLevelEditor draws the editor again as it changes (posted by htmx, as
// the Requirements page's is, with level: the level's id, "" for a new one).
func (p *competitionPages) clubLevelEditor(w http.ResponseWriter, r *http.Request) {
	coach, _, ok := p.coach(w, r)
	if !ok {
		return
	}
	picks, err := p.clubPicks(r, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	own, err := p.clubOwn(r, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	set, problems := parseSetForm(r, picks...)
	applySetAction(&set, r.FormValue("action"), r.FormValue("add"))
	if rule, ok := pickedRule(r.FormValue("addpick"), picks); ok {
		set.Rules = append(set.Rules, rule)
	}
	data := levelEditorData(set, problems, r.PathValue("token"), r.URL.Query().Get("level"), picks, own)
	// Another kind of rule just added opens, to fill in.
	if r.FormValue("add") != "" {
		data.OpenRule = len(data.Set.Rules) - 1
	}
	render(w, r, views.SetEditor(data))
}

// saveNewClubLevel saves the editor's level as a new one of the club's,
// last.
func (p *competitionPages) saveNewClubLevel(w http.ResponseWriter, r *http.Request) {
	p.saveLevelFromEditor(w, r, "")
}

// saveEditedClubLevel saves the editor's level in place of the one it was.
func (p *competitionPages) saveEditedClubLevel(w http.ResponseWriter, r *http.Request) {
	p.saveLevelFromEditor(w, r, r.PathValue("id"))
}

// saveLevelFromEditor saves the posted editor as a level (id, "" for a new
// one); with problems, the editor shows again, saying what.
func (p *competitionPages) saveLevelFromEditor(w http.ResponseWriter, r *http.Request, id string) {
	coach, club, ok := p.coach(w, r)
	if !ok {
		return
	}
	picks, err := p.clubPicks(r, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	set, problems := parseSetForm(r, picks...)
	set.Rules = slices.DeleteFunc(set.Rules, func(rule requirements.Rule) bool { return !trackable(rule) })
	if err := set.Validate(); err != nil || len(problems) > 0 {
		title := "New level"
		if id != "" {
			title = set.Name
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		p.showLevelEditor(w, r, coach, club, title, set, problems, id)
		return
	}
	data, err := json.Marshal(set)
	if err != nil {
		failed(w, r, err)
		return
	}
	token := r.PathValue("token")
	notice := set.Name + " saved."
	if id == "" {
		_, err = p.st.SaveClubLevel(r.Context(), coach, "", set.Name, string(data))
	} else {
		err = p.st.UpdateClubLevel(r.Context(), coach, id, set.Name, string(data))
		// Its club skills are among its rules now, not a list after them.
		if err == nil {
			err = p.st.SetLevelClubItems(r.Context(), coach, id, nil)
		}
	}
	switch {
	case errors.Is(err, store.ErrLimit):
		notice = fmt.Sprintf("The club has %d levels, the most there's room for: remove one first.", store.MaxClubLevels)
	case errors.Is(err, store.ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		failed(w, r, err)
		return
	}
	seeOther(w, r, coachLevelsPath(token), notice)
}

// clubLevel is one of the coach's club's levels, by id.
func (p *competitionPages) clubLevel(r *http.Request, coach store.Coach, id string) (store.ClubLevel, bool, error) {
	levels, err := p.st.ClubLevels(r.Context(), coach.ClubID)
	if err != nil {
		return store.ClubLevel{}, false, err
	}
	i := slices.IndexFunc(levels, func(l store.ClubLevel) bool { return l.ID == id })
	if i < 0 {
		return store.ClubLevel{}, false, nil
	}
	return levels[i], true, nil
}
