package web

import (
	"cmp"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"tariffCalculator/requirements"
	"tariffCalculator/skills"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Club levels (ADR 0011 Decision 5): a club's levels, made of requirements
// in the usual format, written in the requirements editor (the Requirements
// page's Save to club levels posts them here), and the club's own skills to
// track. A gymnast's progress through each is worked out from their marks:
// a skill, a kind of skill or a number of them from the skills marked Can do
// (or Learning); a link or a set routine marked as a whole.

func (p *competitionPages) registerLevels(handle func(string, http.HandlerFunc)) {
	handle("GET /clubs/coach/{token}/levels", p.coachLevels)
	handle("POST /clubs/coach/{token}/levels", p.saveClubLevel)
	handle("POST /clubs/levels/save", p.saveLevelFromLink)
	handle("POST /clubs/coach/{token}/levels/{id}/move", p.moveClubLevel)
	handle("POST /clubs/coach/{token}/levels/{id}/remove", p.removeClubLevel)
	// The level editor on the club's page (web/leveleditor.go).
	handle("GET /clubs/coach/{token}/levels/new", p.newClubLevel)
	handle("POST /clubs/coach/{token}/levels/new", p.saveNewClubLevel)
	handle("GET /clubs/coach/{token}/levels/{id}/edit", p.editClubLevel)
	handle("POST /clubs/coach/{token}/levels/{id}/edit", p.saveEditedClubLevel)
	handle("POST /clubs/coach/{token}/levels/editor", p.clubLevelEditor)
}

// levelItemPrefix starts the item of a level's rule marked as a whole.
const levelItemPrefix = "r:"

// coachLevelsPath is the coaches' page for the club's levels.
func coachLevelsPath(token string) string { return coachPath(token) + "/levels" }

// trackable says a rule can be tracked from a gymnast's skills: a skill, a
// kind of skill, a link, a number of them, or a set routine. Rules about a
// single routine (its length, its difficulty) can't.
func trackable(rule requirements.Rule) bool {
	return slices.Contains([]string{requirements.Includes, requirements.Count, requirements.Separate, requirements.Sequence, requirements.Track}, rule.Type)
}

// describeLevelRule is a club level's rule in words: one of the club's own
// skills by its name (own: the club's skills to track, by id), others as the
// requirements describe them.
func describeLevelRule(rule requirements.Rule, own map[string]store.ClubSkill) string {
	if strings.TrimSpace(rule.Label) == "" {
		switch {
		case rule.Type == requirements.Track:
			if c, ok := own[rule.Item]; ok {
				return c.Name
			}
			return "A club skill no longer kept"
		case requirements.KindOfRule(rule) == "include" && rule.Match != nil:
			// A skill on a level's list is just its name: "Tuck Jump".
			return requirements.Capitalise(requirements.DescribeMatcher(*rule.Match))
		}
	}
	return requirements.Describe(rule)
}

// wholeRule says a rule is marked as a whole, not worked out from the
// skills marked: a set routine, or a link (an option of more than one
// element one straight after another).
func wholeRule(rule requirements.Rule) bool {
	if rule.Type == requirements.Sequence {
		return true
	}
	return rule.Type == requirements.Includes && slices.ContainsFunc(rule.Options, func(o []requirements.Matcher) bool { return len(o) > 1 })
}

// levelItem is the item a rule marked as a whole is kept under: from the
// rule without its wording, so the same link in two levels is one item.
func levelItem(rule requirements.Rule) string {
	rule.Label = ""
	data, _ := json.Marshal(rule)
	sum := sha256.Sum256(data)
	return levelItemPrefix + hex.EncodeToString(sum[:12])
}

// passes says the skills meet a rule, as the requirements engine checks a
// routine, here with no order and repeats allowed.
func passes(rule requirements.Rule, list []skills.TrampolineSkill) bool {
	routine := make([]skills.TrampolineSkill, len(list))
	for i, s := range list {
		s.NormalizePhases()
		s.SetTariff()
		routine[i] = s
	}
	rv := skills.ValidateRoutineWith(routine, skills.ValidateOptions{AllowRepeats: true})
	results := requirements.Evaluate(requirements.Set{Rules: []requirements.Rule{rule}}, rv)
	return len(results) == 1 && results[0].Passed
}

// levelsProgress is how far a member is through each of the club's levels,
// one for each level in order: each item with how it stands on each side
// (fill gives a row its marks), from their marks (t), with the kept routines
// that already do a whole item.
func levelsProgress(levels []store.ClubLevel, clubItems map[string]store.ClubSkill, t tracking, kept []store.Routine, gymnast string, fill func(*views.MarkView, map[string]store.Mark)) []views.LevelView {
	var routines []struct {
		name  string
		skill []skills.TrampolineSkill
	}
	for _, k := range kept {
		if list, err := parseRoutineJSON(k.Skills); err == nil {
			routines = append(routines, struct {
				name  string
				skill []skills.TrampolineSkill
			}{k.Name, list})
		}
	}
	derived := func(side string, rule requirements.Rule) string {
		can, learning := t.sideSkills[side][store.CanDo], t.sideSkills[side][store.Learning]
		switch {
		case passes(rule, can):
			return store.CanDo
		case len(learning) > 0 && passes(rule, append(slices.Clone(can), learning...)):
			return store.Learning
		}
		return ""
	}
	var out []views.LevelView
	for _, l := range levels {
		lv := views.LevelView{Name: l.Name}
		set, err := requirements.Parse([]byte(l.Set))
		if err != nil {
			lv.Broken = "This level's requirements couldn't be read."
			out = append(out, lv)
			continue
		}
		// The club skills it lists after its rules (from before they could be
		// rules) come last, as if they were.
		for _, rule := range withClubItems(set.Rules, l.ClubItems) {
			if !trackable(rule) {
				continue
			}
			if rule.Type == requirements.Track {
				c, ok := clubItems[store.ClubSkill{ID: rule.Item}.Item()]
				if !ok {
					continue
				}
				v := views.MarkView{Club: c.ID, Name: c.Name, FIG: c.Note}
				if strings.TrimSpace(rule.Label) != "" {
					v.Name = rule.Label
				}
				fill(&v, t.sides(c.Item()))
				lv.Items = append(lv.Items, v)
				continue
			}
			v := views.MarkView{Name: describeLevelRule(rule, nil)}
			if wholeRule(rule) {
				v.Item = levelItem(rule)
				fill(&v, t.levelMarks[v.Item])
				var in []string
				for _, k := range routines {
					rv := skills.ValidateRoutineWith(k.skill, skills.ValidateOptions{AllowRepeats: true})
					if res := requirements.Evaluate(requirements.Set{Rules: []requirements.Rule{rule}}, rv); len(res) == 1 && res[0].Passed {
						in = append(in, k.name)
					}
				}
				if len(in) > 0 {
					v.FIG = "Done in " + strings.Join(in, ", ")
				}
			} else {
				v.Derived, v.FIG = true, "From the skills marked"
				if state := derived(store.SideGymnast, rule); state != "" {
					v.Gymnast = &views.SideMark{State: state, By: gymnast}
				}
				if state := derived(store.SideCoach, rule); state != "" {
					v.Coach = &views.SideMark{State: state}
				}
			}
			lv.Items = append(lv.Items, v)
		}
		for _, v := range lv.Items {
			if v.Gymnast != nil && v.Gymnast.State == store.CanDo {
				lv.Gymnast++
			}
			if v.Coach != nil && v.Coach.State == store.CanDo {
				lv.Coach++
			}
		}
		out = append(out, lv)
	}
	return out
}

// levelWholeItem is a club level's rule marked as a whole, by its item, with
// its wording; ok false if none of the club's levels has it.
func (p *competitionPages) levelWholeItem(r *http.Request, clubID, item string) (requirements.Rule, bool, error) {
	levels, err := p.st.ClubLevels(r.Context(), clubID)
	if err != nil {
		return requirements.Rule{}, false, err
	}
	for _, l := range levels {
		set, err := requirements.Parse([]byte(l.Set))
		if err != nil {
			continue
		}
		for _, rule := range set.Rules {
			if trackable(rule) && wholeRule(rule) && levelItem(rule) == item {
				return rule, true, nil
			}
		}
	}
	return requirements.Rule{}, false, nil
}

// coachLevels is the coaches' page for the club's levels.
func (p *competitionPages) coachLevels(w http.ResponseWriter, r *http.Request) {
	coach, club, ok := p.coach(w, r)
	if !ok {
		return
	}
	levels, err := p.st.ClubLevels(r.Context(), coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	_, other, err := p.clubSkillsOf(r, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	own := map[string]store.ClubSkill{}
	for _, c := range other {
		own[c.ID] = c
	}
	base := coachLevelsPath(r.PathValue("token"))
	page := views.ClubLevelsPage{Frame: coachFrame(r, coach, club, coachLevelsTab), Max: store.MaxClubLevels, New: base + "/new"}
	for _, g := range requirements.BuiltinGroups() {
		group := views.SetRoutineGroup{Name: g.Name}
		for _, b := range g.SetRoutines() {
			group.Routines = append(group.Routines, views.SetRoutineChoice{Name: b.Set.Name, Href: base + "/new?set=" + url.QueryEscape(requirements.BuiltinPrefix+b.ID)})
		}
		if len(group.Routines) > 0 {
			page.Sets = append(page.Sets, group)
		}
	}
	for i, l := range levels {
		v := views.ClubLevelView{Name: l.Name, By: l.ByName, Updated: l.UpdatedAt.In(local).Format("2 January 2006"),
			Move: base + "/" + l.ID + "/move", Remove: base + "/" + l.ID + "/remove",
			First: i == 0, Last: i == len(levels)-1}
		if set, err := requirements.Parse([]byte(l.Set)); err == nil {
			for _, rule := range withClubItems(set.Rules, l.ClubItems) {
				v.Rules = append(v.Rules, describeLevelRule(rule, own))
			}
			v.Edit, v.Copy = base+"/"+l.ID+"/edit", base+"/new?from="+l.ID
			v.Open = requirementsLink(l.Set, cmp.Or(l.Source, l.ID))
			v.Share = levelShareLink(r, l, own)
		} else {
			v.Broken = "Its requirements couldn't be read."
		}
		page.Levels = append(page.Levels, v)
	}
	render(w, r, views.CoachClubLevels(page))
}

// requirementsLink opens the Requirements page with requirements to add, as
// a share link (static/js/share.js) does, so they can be edited and saved to
// the club again.
func requirementsLink(set, id string) string {
	// The id goes with it, so Save to club levels there changes this level
	// rather than adding another (store.SaveClubLevel).
	data, err := json.Marshal(map[string]any{"v": 1, "sets": []json.RawMessage{json.RawMessage(set)}, "ids": []string{id}})
	if err != nil {
		return ""
	}
	return "/requirements#share=j" + base64.RawURLEncoding.EncodeToString(data)
}

// saveClubLevel is a coach saving requirements as one of the club's levels, from
// the Requirements page: set (the requirements as JSON) and id (the
// browser's id for them). Rules that can't be tracked from skills are left
// out, saying so.
func (p *competitionPages) saveClubLevel(w http.ResponseWriter, r *http.Request) {
	coach, _, ok := p.coach(w, r)
	if !ok {
		return
	}
	p.saveLevelAs(w, r, coach, r.PathValue("token"))
}

// saveLevelFromLink is Save to club levels from the Requirements page by any
// of a person's own links (link): a coach's, or a member's whose person
// coaches too (ADR 0010), as coachFromLink finds the coach.
func (p *competitionPages) saveLevelFromLink(w http.ResponseWriter, r *http.Request) {
	coach, token, ok := p.coachFromLink(w, r, "/clubs/levels/save", []string{"set", "id"})
	if !ok {
		return
	}
	p.saveLevelAs(w, r, coach, token)
}

// saveLevelAs saves the posted requirements as one of the coach's club's
// levels, and opens the Levels page their link (token) opens.
func (p *competitionPages) saveLevelAs(w http.ResponseWriter, r *http.Request, coach store.Coach, token string) {
	set, err := requirements.Parse([]byte(r.FormValue("set")))
	if err != nil {
		badRequest(w, fmt.Errorf("those requirements couldn't be read: %w", err))
		return
	}
	var rules []requirements.Rule
	for _, rule := range set.Rules {
		if trackable(rule) {
			rules = append(rules, rule)
		}
	}
	left := len(set.Rules) - len(rules)
	set.Rules = rules
	data, err := json.Marshal(set)
	if err != nil {
		badRequest(w, err)
		return
	}
	source := r.FormValue("id")
	if len(source) > 40 {
		source = ""
	}
	added, err := p.st.SaveClubLevel(r.Context(), coach, source, set.Name, string(data))
	if errors.Is(err, store.ErrLimit) {
		seeOther(w, r, coachLevelsPath(token), fmt.Sprintf("The club has %d levels, the most there's room for: remove one first.", store.MaxClubLevels))
		return
	} else if err != nil {
		failed(w, r, err)
		return
	}
	notice := "Saved " + set.Name + "."
	if added {
		notice = "Added " + set.Name + " to the club's levels."
	}
	if left > 0 {
		notice += fmt.Sprintf(" %s about a single routine (its length, difficulty or the like) can't be tracked from skills, so %s left out.", plural(left, "rule", "rules"), map[bool]string{true: "it was", false: "they were"}[left == 1])
	}
	seeOther(w, r, coachLevelsPath(token), notice)
}

// moveClubLevel moves a level one place earlier (by=-1) or later (by=1).
func (p *competitionPages) moveClubLevel(w http.ResponseWriter, r *http.Request) {
	coach, _, ok := p.coach(w, r)
	if !ok {
		return
	}
	by := 1
	if r.FormValue("by") == "-1" {
		by = -1
	}
	if err := p.st.MoveClubLevel(r.Context(), coach, r.PathValue("id"), by); err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, coachLevelsPath(r.PathValue("token")), "")
}

// removeClubLevel removes one of the club's levels.
func (p *competitionPages) removeClubLevel(w http.ResponseWriter, r *http.Request) {
	coach, _, ok := p.coach(w, r)
	if !ok {
		return
	}
	if err := p.st.RemoveClubLevel(r.Context(), coach, r.PathValue("id")); err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, coachLevelsPath(r.PathValue("token")), "Removed.")
}

// coachFromLink is the coach a person's own link (the form's link) stands
// for: a coach's link is theirs; a member's (or one entering on their own)
// is the coach the same person is (ADR 0010), opened through it. Where the
// person coaches in more than one club, a page asks which (the form's role,
// a coach's id), posting the form's fields (keep) back to action. It gives
// the coach and the link that opens their pages.
func (p *competitionPages) coachFromLink(w http.ResponseWriter, r *http.Request, action string, keep []string) (store.Coach, string, bool) {
	kind, token, ok := pastedLink(r.FormValue("link"))
	if !ok || (kind != store.RoleCoach && kind != store.RoleMember && kind != store.RoleIndividual) {
		message(w, r, http.StatusBadRequest, "That isn't your own link", "Give the link of your own page: a coach's, or a member's if you coach in the club too.")
		return store.Coach{}, "", false
	}
	if kind == store.RoleCoach && !store.IsVia(token) {
		c, err := p.st.CoachByLink(r.Context(), token)
		if err != nil {
			failed(w, r, err)
			return c, "", false
		}
		return c, token, true
	}
	_, roles, err := p.st.Roles(r.Context(), token)
	if err != nil {
		failed(w, r, err)
		return store.Coach{}, "", false
	}
	var coaches []store.Role
	for _, role := range roles {
		if role.Kind == store.RoleCoach {
			coaches = append(coaches, role)
		}
	}
	pick := slices.IndexFunc(coaches, func(role store.Role) bool { return role.ID == r.FormValue("role") })
	switch {
	case len(coaches) == 0:
		message(w, r, http.StatusConflict, "You're not a coach here", "That link's page has no coach's role. Your club's comp sec can make you a coach too, or use your coach page's link.")
		return store.Coach{}, "", false
	case len(coaches) == 1:
		pick = 0
	case pick < 0:
		page := views.ChooseCoachPage{Action: action, Link: r.FormValue("link")}
		for _, name := range keep {
			page.Keep = append(page.Keep, views.FormValue{Name: name, Value: r.FormValue(name)})
		}
		for _, role := range coaches {
			page.Choices = append(page.Choices, views.FormValue{Name: role.Place, Value: role.ID})
		}
		render(w, r, views.ChooseCoach(page))
		return store.Coach{}, "", false
	}
	via := store.Via(store.Personal(token), coaches[pick].ID)
	if coaches[pick].Own {
		via = store.Personal(token)
	}
	c, err := p.st.CoachByLink(r.Context(), via)
	if err != nil {
		failed(w, r, err)
		return c, "", false
	}
	return c, via, true
}

// Where a member stands on a club level, as the Level filter's values have
// it after the level's id ("<id>:done").
const (
	levelDone    = "done"    // every item marked Can do by a coach
	levelWorking = "working" // something marked on either side, not done yet
)

// levelStandings are where each of a club's members with marks stands on
// each of its levels (levels), by member, as the Level filter's values.
func (p *competitionPages) levelStandings(r *http.Request, clubID string, levels []store.ClubLevel) (map[string][]string, error) {
	out := map[string][]string{}
	if len(levels) == 0 {
		return out, nil
	}
	marks, err := p.st.ClubMarks(r.Context(), clubID)
	if err != nil {
		return nil, err
	}
	_, other, err := p.clubSkillsOf(r, clubID)
	if err != nil {
		return nil, err
	}
	clubItems := map[string]store.ClubSkill{}
	for _, c := range other {
		clubItems[c.Item()] = c
	}
	fill := func(v *views.MarkView, sides map[string]store.Mark) {
		if k, ok := sides[store.SideGymnast]; ok {
			v.Gymnast = &views.SideMark{State: k.State}
		}
		if k, ok := sides[store.SideCoach]; ok {
			v.Coach = &views.SideMark{State: k.State}
		}
	}
	for member, list := range marks {
		for i, lv := range levelsProgress(levels, clubItems, trackingOf(list, clubItems), nil, "", fill) {
			if len(lv.Items) == 0 {
				continue
			}
			started := slices.ContainsFunc(lv.Items, func(v views.MarkView) bool { return v.Gymnast != nil || v.Coach != nil })
			switch {
			case lv.Coach == len(lv.Items):
				out[member] = append(out[member], levels[i].ID+":"+levelDone)
			case started:
				out[member] = append(out[member], levels[i].ID+":"+levelWorking)
			}
		}
	}
	return out, nil
}

// levelFilter is a filter by where members stand on the club's levels:
// completed (by a coach's marks) or working on, for each level.
func levelFilter(levels []store.ClubLevel) views.ListFilter {
	f := views.ListFilter{Key: "level", Label: "Level", Any: "Any level"}
	for _, l := range levels {
		f.Options = append(f.Options, views.ListOption{Value: l.ID + ":" + levelDone, Label: l.Name + ": completed", Group: l.Name},
			views.ListOption{Value: l.ID + ":" + levelWorking, Label: l.Name + ": working on", Group: l.Name})
	}
	return f
}
