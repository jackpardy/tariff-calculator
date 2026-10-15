package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"tariffCalculator/catalog"
	"tariffCalculator/skills"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Skill tracking (ADR 0011 Decisions 2 and 3): a member's Skills tab, where
// they mark what they're learning and can do; a coach's page for each
// gymnast, to mark them too and confirm the gymnast's own; the comp sec's
// view; and, where the club shares them, other members' skills.

func (p *competitionPages) registerSkills(handle func(string, http.HandlerFunc)) {
	handle("GET /clubs/member/{token}/skills", p.memberSkills)
	handle("POST /clubs/member/{token}/skills", p.markOwnSkill)
	handle("GET /clubs/member/{token}/skills/{member}", p.otherMemberSkills)
	handle("GET /clubs/coach/{token}/gymnasts/{member}/skills", p.coachGymnastSkills)
	handle("POST /clubs/coach/{token}/gymnasts/{member}/skills", p.markGymnastSkill)
	handle("GET /clubs/admin/{token}/members/{member}/skills", p.clubMemberSkills)
	handle("POST /clubs/admin/{token}/skills-open", p.skillsOpen)
}

// memberSkillsPath is a member's Skills tab.
func memberSkillsPath(token string) string { return memberPath(token) + "/skills" }

// withQuery is a path with the search being made on it, if there is one.
func withQuery(path, q string) string {
	if q == "" {
		return path
	}
	return path + "?q=" + url.QueryEscape(q)
}

// skillViewer is who's looking at a member's skills: the gymnast, a coach
// (who marks as themselves), or someone only looking (the comp sec, or
// another member).
type skillViewer struct {
	gymnast bool
	coach   *store.Coach
}

// memberSkills is a member's Skills tab.
func (p *competitionPages) memberSkills(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	page, _, ok := p.memberPage(w, r, m, memberSkillsTab)
	if !ok {
		return
	}
	token, q := r.PathValue("token"), r.URL.Query().Get("q")
	part, err := p.skillsPart(r, m, skillViewer{gymnast: true}, withQuery(memberSkillsPath(token), q), memberSkillsPath(token), q)
	if err != nil {
		failed(w, r, err)
		return
	}
	club, err := p.st.ClubByID(r.Context(), m.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	if part.Open = club.SkillsOpen; club.SkillsOpen {
		members, err := p.st.Members(r.Context(), m.ClubID)
		if err != nil {
			failed(w, r, err)
			return
		}
		can, _, err := p.st.MarkCounts(r.Context(), m.ClubID)
		if err != nil {
			failed(w, r, err)
			return
		}
		for _, o := range members {
			if o.ID != m.ID {
				part.Others = append(part.Others, views.OtherMember{Name: o.Name, Open: memberSkillsPath(token) + "/" + o.ID, Can: can[o.ID]})
			}
		}
	}
	page.Skills = part
	render(w, r, views.MemberSkills(page))
}

// markOwnSkill is a member marking a skill (skill: its JSON; state: learning,
// can, or "" to take it off).
func (p *competitionPages) markOwnSkill(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	in, name, ok := postedMark(w, r)
	if !ok {
		return
	}
	if err := p.st.MarkAsGymnast(r.Context(), m, in); err != nil {
		markFailed(w, r, err)
		return
	}
	seeOther(w, r, withQuery(memberSkillsPath(r.PathValue("token")), r.URL.Query().Get("q")), markNotice(name, in.State))
}

// otherMemberSkills is another member's skills, where the club shares them.
func (p *competitionPages) otherMemberSkills(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	club, err := p.st.ClubByID(r.Context(), m.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	other, err := p.st.Member(r.Context(), m.ClubID, r.PathValue("member"))
	if err != nil || !club.SkillsOpen || other.ID == m.ID {
		failed(w, r, store.ErrNotFound)
		return
	}
	page, _, ok := p.memberPage(w, r, m, memberSkillsTab)
	if !ok {
		return
	}
	part, err := p.skillsPart(r, other, skillViewer{}, "", "", "")
	if err != nil {
		failed(w, r, err)
		return
	}
	render(w, r, views.OtherMemberSkills(page, views.SkillsViewPage{Back: memberSkillsPath(r.PathValue("token")), BackTo: "Your skills", Name: other.Name, Skills: part}))
}

// coachGymnastSkills is a coach's page for a gymnast's skills.
func (p *competitionPages) coachGymnastSkills(w http.ResponseWriter, r *http.Request) {
	coach, club, ok := p.coach(w, r)
	if !ok {
		return
	}
	m, err := p.st.CoachMember(r.Context(), coach, r.PathValue("member"))
	if err != nil {
		failed(w, r, err)
		return
	}
	base, q := coachGymnastsPath(r.PathValue("token"))+"/"+m.ID, r.URL.Query().Get("q")
	part, err := p.skillsPart(r, m, skillViewer{coach: &coach}, withQuery(base+"/skills", q), base+"/skills", q)
	if err != nil {
		failed(w, r, err)
		return
	}
	render(w, r, views.CoachGymnastSkills(coachFrame(r, coach, club, coachGymnastsTab), views.SkillsViewPage{Back: base, BackTo: m.Name + "'s routines", Name: m.Name, Skills: part}))
}

// markGymnastSkill is a coach marking a skill for a gymnast they see.
func (p *competitionPages) markGymnastSkill(w http.ResponseWriter, r *http.Request) {
	coach, _, ok := p.coach(w, r)
	if !ok {
		return
	}
	in, name, ok := postedMark(w, r)
	if !ok {
		return
	}
	member := r.PathValue("member")
	if err := p.st.MarkAsCoach(r.Context(), coach, member, in); err != nil {
		markFailed(w, r, err)
		return
	}
	seeOther(w, r, withQuery(coachGymnastsPath(r.PathValue("token"))+"/"+member+"/skills", r.URL.Query().Get("q")), markNotice(name, in.State))
}

// clubMemberSkills shows the comp sec a member's skills.
func (p *competitionPages) clubMemberSkills(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	m, err := p.st.Member(r.Context(), club.ID, r.PathValue("member"))
	if err != nil {
		failed(w, r, err)
		return
	}
	part, err := p.skillsPart(r, m, skillViewer{}, "", "", "")
	if err != nil {
		failed(w, r, err)
		return
	}
	token := r.PathValue("token")
	render(w, r, views.ClubMemberSkills(views.ClubPage{Base: clubPath(token), Name: club.Name, Menu: clubMenu(token, clubMembers)},
		views.SkillsViewPage{Back: clubMembersPath(token), BackTo: "Members", Name: m.Name, Skills: part}))
}

// skillsOpen is the comp sec letting every member see everyone's skills, or not.
func (p *competitionPages) skillsOpen(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	on := r.FormValue("on") == "1"
	if err := p.st.SetSkillsOpen(r.Context(), club.ID, on); err != nil {
		failed(w, r, err)
		return
	}
	notice := "Each member's skills are seen by them, their coaches and you."
	if on {
		notice = "Every member now sees everyone's skills."
	}
	seeOther(w, r, clubSettingsPath(r.PathValue("token")), notice)
}

// postedMark is the mark posted: the skill (as JSON, checked as the builder
// checks a skill) and its state, with the skill's name for the notice.
func postedMark(w http.ResponseWriter, r *http.Request) (store.MarkIn, string, bool) {
	routine, err := parseRoutineJSON("[" + r.FormValue("skill") + "]")
	if err != nil || len(routine) != 1 {
		badRequest(w, errors.New("that skill couldn't be read"))
		return store.MarkIn{}, "", false
	}
	s := routine[0]
	s.Name = skills.FindCommonSkillName(s)
	data, err := json.Marshal(s)
	if err != nil {
		badRequest(w, err)
		return store.MarkIn{}, "", false
	}
	state := r.FormValue("state")
	if state != "" && state != store.Learning && state != store.CanDo {
		badRequest(w, errors.New("mark it Learning or Can do"))
		return store.MarkIn{}, "", false
	}
	return store.MarkIn{Skill: string(data), State: state}, skillName(s), true
}

// markFailed reports a mark that couldn't be made.
func markFailed(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrLimit) {
		message(w, r, http.StatusConflict, "Too many skills", fmt.Sprintf("A member can have up to %d skills marked. Take some off first.", store.MaxMarks))
		return
	}
	failed(w, r, err)
}

// markNotice says what a mark did.
func markNotice(name, state string) string {
	switch state {
	case store.CanDo:
		return name + ": Can do."
	case store.Learning:
		return name + ": Learning."
	}
	return name + " taken off."
}

// skillName is a skill's name, or its notation where it has no name.
func skillName(s skills.TrampolineSkill) string {
	if name := skills.FindCommonSkillName(s); name != "Custom Skill" {
		return name
	}
	return "Skill " + s.FIGNotation()
}

// skillsPart is a member's skills as viewer sees them: their marks, and, for
// the gymnast and their coaches, skills to mark (action: where marks post;
// search: where searching goes; q: the search).
func (p *competitionPages) skillsPart(r *http.Request, m store.Member, viewer skillViewer, action, search, q string) (views.SkillsPart, error) {
	ctx := r.Context()
	marks, err := p.st.MemberMarks(ctx, m.ID)
	if err != nil {
		return views.SkillsPart{}, err
	}
	part := views.SkillsPart{Whose: m.Name + "'s", Action: action, Search: search, Query: q, Coach: viewer.coach != nil}
	if viewer.gymnast {
		part.Whose = "your"
	}
	type markedSkill struct {
		skill skills.TrampolineSkill
		mark  store.Mark
	}
	var marked []markedSkill
	for _, k := range marks {
		var s skills.TrampolineSkill
		if json.Unmarshal([]byte(k.Skill), &s) != nil {
			continue
		}
		marked = append(marked, markedSkill{s, k})
		v := views.MarkView{Skill: k.Skill, Name: skillName(s), FIG: s.FIGNotation(), State: k.State, ByGymnast: k.ByGymnast(), At: k.At.In(local).Format("2 Jan")}
		switch {
		case k.ByGymnast() && viewer.gymnast:
			v.By = "You"
		case k.ByGymnast():
			v.By = m.Name
		case viewer.coach != nil && k.ByCoach == viewer.coach.ID:
			v.By = "You (coach)"
		default:
			v.By = k.ByName + " (coach)"
		}
		if viewer.coach != nil && k.ByGymnast() {
			v.ToConfirm = true
			part.ToConfirm++
		}
		if k.State == store.CanDo {
			part.Can = append(part.Can, v)
		} else {
			part.Learning = append(part.Learning, v)
		}
	}
	if action == "" {
		return part, nil
	}
	choice := func(s skills.TrampolineSkill) views.SkillChoice {
		s.SetTariff()
		data, _ := json.Marshal(s)
		c := views.SkillChoice{Skill: string(data), Name: skillName(s), FIG: s.FIGNotation(), Tariff: fmt.Sprintf("%.1f", s.Tariff)}
		for _, k := range marked {
			if k.skill.Equal(&s) {
				c.State, c.ByGymnast = k.mark.State, k.mark.ByGymnast()
				break
			}
		}
		return c
	}
	for _, res := range catalog.Search(q) {
		part.Results = append(part.Results, choice(res.Skill))
	}
	kept, err := p.st.MemberRoutines(ctx, m.ID)
	if err != nil {
		return views.SkillsPart{}, err
	}
	var seen []skills.TrampolineSkill
	for _, k := range kept {
		routine, err := parseRoutineJSON(k.Skills)
		if err != nil {
			continue
		}
		for _, s := range routine {
			if !slices.ContainsFunc(seen, func(o skills.TrampolineSkill) bool { return o.Equal(&s) }) {
				seen = append(seen, s)
				part.Routines = append(part.Routines, choice(s))
			}
		}
	}
	for _, c := range catalog.Categories() {
		g := views.SkillGroup{Title: c.Label}
		var entries []catalog.Entry
		for _, group := range c.Groups {
			entries = append(entries, group.Options...)
		}
		for _, e := range append(entries, c.Entries...) {
			shapes := e.Shapes()
			if len(shapes) == 0 {
				g.Skills = append(g.Skills, choice(e.Skill))
				continue
			}
			for _, shape := range shapes {
				s := e.Skill
				s.Shape = shape
				g.Skills = append(g.Skills, choice(s))
			}
		}
		part.Browse = append(part.Browse, g)
	}
	return part, nil
}
