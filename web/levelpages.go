package web

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	handle("POST /clubs/coach/{token}/levels/{id}/items", p.clubLevelItems)
	handle("POST /clubs/coach/{token}/levels/{id}/move", p.moveClubLevel)
	handle("POST /clubs/coach/{token}/levels/{id}/remove", p.removeClubLevel)
}

// levelItemPrefix starts the item of a level's rule marked as a whole.
const levelItemPrefix = "r:"

// coachLevelsPath is the coaches' page for the club's levels.
func coachLevelsPath(token string) string { return coachPath(token) + "/levels" }

// trackable says a rule can be tracked from a gymnast's skills: a skill, a
// kind of skill, a link, a number of them, or a set routine. Rules about a
// single routine (its length, its difficulty) can't.
func trackable(rule requirements.Rule) bool {
	return slices.Contains([]string{requirements.Includes, requirements.Count, requirements.Separate, requirements.Sequence}, rule.Type)
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

// levelsProgress is how far a member is through each of the club's levels:
// each item with how it stands on each side (fill gives a row its marks),
// from their marks on whole items (levelMarks) and club skills (itemSides),
// and the skills each side marked (sideSkills, by side and state), with the
// kept routines that already do a whole item.
func levelsProgress(levels []store.ClubLevel, clubItems map[string]store.ClubSkill, levelMarks, itemSides map[string]map[string]store.Mark,
	sideSkills map[string]map[string][]skills.TrampolineSkill, kept []store.Routine, gymnast string, fill func(*views.MarkView, map[string]store.Mark)) []views.LevelView {
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
		can := sideSkills[side][store.CanDo]
		switch {
		case passes(rule, can):
			return store.CanDo
		case passes(rule, append(slices.Clone(can), sideSkills[side][store.Learning]...)):
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
		for _, rule := range set.Rules {
			if !trackable(rule) {
				continue
			}
			v := views.MarkView{Name: requirements.Describe(rule)}
			if wholeRule(rule) {
				v.Item = levelItem(rule)
				fill(&v, levelMarks[v.Item])
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
		for _, id := range l.ClubItems {
			c, ok := clubItems[store.ClubSkill{ID: id}.Item()]
			if !ok {
				continue
			}
			v := views.MarkView{Club: c.ID, Name: c.Name, FIG: c.Note}
			fill(&v, itemSides[c.Item()])
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
	page := views.ClubLevelsPage{Frame: coachFrame(r, coach, club, coachLevelsTab), Max: store.MaxClubLevels}
	for i, l := range levels {
		v := views.ClubLevelView{Name: l.Name, By: l.ByName, Updated: l.UpdatedAt.In(local).Format("2 January 2006"),
			Items: base + "/" + l.ID + "/items", Move: base + "/" + l.ID + "/move", Remove: base + "/" + l.ID + "/remove",
			First: i == 0, Last: i == len(levels)-1}
		if set, err := requirements.Parse([]byte(l.Set)); err == nil {
			for _, rule := range set.Rules {
				v.Rules = append(v.Rules, requirements.Describe(rule))
			}
			v.Open = requirementsLink(l.Set)
			v.Share = levelShareLink(r, l, own)
		} else {
			v.Broken = "Its requirements couldn't be read."
		}
		for _, c := range other {
			v.ClubSkills = append(v.ClubSkills, views.LevelClubSkill{ID: c.ID, Name: c.Name, In: slices.Contains(l.ClubItems, c.ID)})
		}
		page.Levels = append(page.Levels, v)
	}
	render(w, r, views.CoachClubLevels(page))
}

// requirementsLink opens the Requirements page with requirements to add, as
// a share link (static/js/share.js) does, so they can be edited and saved to
// the club again.
func requirementsLink(set string) string {
	data, err := json.Marshal(map[string]any{"v": 1, "sets": []json.RawMessage{json.RawMessage(set)}})
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

// clubLevelItems are the club skills a level tracks besides its requirements
// (item: each one ticked, by id).
func (p *competitionPages) clubLevelItems(w http.ResponseWriter, r *http.Request) {
	coach, _, ok := p.coach(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	if err := p.st.SetLevelClubItems(r.Context(), coach, r.PathValue("id"), r.Form["item"]); err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, coachLevelsPath(r.PathValue("token")), "Saved.")
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
