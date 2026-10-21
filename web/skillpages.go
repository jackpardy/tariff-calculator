package web

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"tariffCalculator/catalog"
	"tariffCalculator/requirements"
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
	handle("GET /clubs/coach/{token}/club-skills", p.clubSkillsPage)
	handle("POST /clubs/coach/{token}/club-skills", p.addClubSkill)
	handle("POST /clubs/coach/{token}/club-skills/{id}/remove", p.removeClubSkill)
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
	in, name, ok := p.postedMark(w, r, m.ClubID)
	if !ok {
		return
	}
	if err := p.st.MarkAsGymnast(r.Context(), m, in); err != nil {
		markFailed(w, r, err)
		return
	}
	seeOther(w, r, withQuery(memberSkillsPath(r.PathValue("token")), r.URL.Query().Get("q"))+markAt(r), markNotice(name, in.State))
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
	render(w, r, views.CoachGymnastSkills(coachFrame(r, coach, club, coachGymnastsTab, nil), views.SkillsViewPage{Back: base, BackTo: m.Name + "'s routines", Name: m.Name, Skills: part}))
}

// markGymnastSkill is a coach marking a skill for a gymnast they see.
func (p *competitionPages) markGymnastSkill(w http.ResponseWriter, r *http.Request) {
	coach, _, ok := p.coach(w, r)
	if !ok {
		return
	}
	in, name, ok := p.postedMark(w, r, coach.ClubID)
	if !ok {
		return
	}
	member := r.PathValue("member")
	if err := p.st.MarkAsCoach(r.Context(), coach, member, in); err != nil {
		markFailed(w, r, err)
		return
	}
	if back := r.FormValue("back"); routineAnchorRE.MatchString(back) {
		// Marked on the gymnast's page, by a routine: back there.
		seeOther(w, r, coachGymnastsPath(r.PathValue("token"))+"/"+member+"#"+back, markNotice(name, in.State))
		return
	}
	seeOther(w, r, withQuery(coachGymnastsPath(r.PathValue("token"))+"/"+member+"/skills", r.URL.Query().Get("q"))+markAt(r), markNotice(name, in.State))
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
	render(w, r, views.ClubMemberSkills(views.ClubPage{Base: clubPath(token), Name: club.Name, Menu: clubMenu(r, clubMembers)},
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

// markAt is the anchor of the skill row a mark was posted from, to come
// back to it ("" for none, or one that isn't a skill row's).
func markAt(r *http.Request) string {
	if at := r.FormValue("at"); markAnchorRE.MatchString(at) {
		return "#" + at
	}
	return ""
}

var (
	markAnchorRE    = regexp.MustCompile(`^mark-[a-z0-9]+-[0-9a-f]{8}$`)
	routineAnchorRE = regexp.MustCompile(`^routine-[A-Za-z0-9_-]+$`)
)

// postedMark is the mark posted: the skill (as JSON, checked as the builder
// checks a skill), or one of the club's own skills to track (club: its id),
// and its state, with the skill's name for the notice.
func (p *competitionPages) postedMark(w http.ResponseWriter, r *http.Request, clubID string) (store.MarkIn, string, bool) {
	state := r.FormValue("state")
	if state != "" && state != store.Learning && state != store.CanDo {
		badRequest(w, errors.New("mark it Learning or Can do"))
		return store.MarkIn{}, "", false
	}
	named, other, err := p.clubSkillsOf(r, clubID)
	if err != nil {
		failed(w, r, err)
		return store.MarkIn{}, "", false
	}
	if item := r.FormValue("item"); item != "" {
		rule, ok, err := p.levelWholeItem(r, clubID, item)
		if err != nil || !ok {
			failed(w, r, cmp.Or(err, store.ErrNotFound))
			return store.MarkIn{}, "", false
		}
		data, _ := json.Marshal(rule)
		return store.MarkIn{Item: item, Skill: string(data), State: state}, requirements.Describe(rule), true
	}
	if id := r.FormValue("club"); id != "" {
		for _, c := range other {
			if c.ID == id {
				return store.MarkIn{Item: c.Item(), State: state}, c.Name, true
			}
		}
		failed(w, r, store.ErrNotFound)
		return store.MarkIn{}, "", false
	}
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
	return store.MarkIn{Skill: string(data), State: state}, namedAs(named, s), true
}

// clubSkillsOf are a club's own skills: its names for catalog skills, as
// the skills with the names as their custom names, and the rest, to track.
func (p *competitionPages) clubSkillsOf(r *http.Request, clubID string) (named []skills.TrampolineSkill, other []store.ClubSkill, err error) {
	list, err := p.st.ClubSkills(r.Context(), clubID)
	if err != nil {
		return nil, nil, err
	}
	for _, c := range list {
		if c.Skill == "" {
			other = append(other, c)
			continue
		}
		var s skills.TrampolineSkill
		if json.Unmarshal([]byte(c.Skill), &s) == nil {
			named = append(named, s)
		}
	}
	return named, other, nil
}

// clubName is the club's name for a skill (found by
// TrampolineSkill.Equal), "" if it hasn't named it.
func clubName(named []skills.TrampolineSkill, s skills.TrampolineSkill) string {
	for _, n := range named {
		if n.Equal(&s) {
			return n.CustomName
		}
	}
	return ""
}

// namedAs is a skill's name at the club: the club's name for it, or its own.
func namedAs(named []skills.TrampolineSkill, s skills.TrampolineSkill) string {
	if name := clubName(named, s); name != "" {
		return name
	}
	return skillName(s)
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

// skillName is a skill's name, or where it has no name its notation and
// what tells it from others with the same: where it takes off and lands, and
// which way it turns, e.g. "Skill (4 1 /): from back, forward".
func skillName(s skills.TrampolineSkill) string {
	if name := skills.FindCommonSkillName(s); name != skills.UnnamedSkill {
		return name
	}
	var how []string
	if s.TakeoffPosition != skills.Feet {
		how = append(how, "from "+strings.ToLower(skills.BodyPositionName[s.TakeoffPosition]))
	}
	if s.Rotation > 0 {
		how = append(how, map[bool]string{true: "backward", false: "forward"}[s.Backward])
	}
	if l := s.LandingPosition(); l != skills.Feet {
		how = append(how, "to "+strings.ToLower(skills.BodyPositionName[l]))
	}
	if len(how) == 0 {
		return "Skill " + s.FIGNotation()
	}
	return "Skill " + s.FIGNotation() + ": " + strings.Join(how, ", ")
}

// skillNameBeside is skillName where the notation shows beside it, so it
// isn't said twice: "Skill: from back, forward".
func skillNameBeside(s skills.TrampolineSkill) string {
	if s.FIGNotation() == "" {
		return skillName(s) // a seat drop: nothing to take out ("SeatDrop" otherwise)
	}
	return strings.Replace(skillName(s), " "+s.FIGNotation(), "", 1)
}

// skillWithNotation is a skill's name and its notation, once: an unnamed
// skill's name already has it.
func skillWithNotation(s skills.TrampolineSkill) string {
	if skills.FindCommonSkillName(s) == skills.UnnamedSkill {
		return skillName(s)
	}
	return skillName(s) + " " + s.FIGNotation()
}

// skillsPart is a member's skills as viewer sees them: their marks, both
// sides, by how they stand, and, for the gymnast and their coaches, skills to
// mark (action: where marks post; search: where searching goes; q: the
// search).
func (p *competitionPages) skillsPart(r *http.Request, m store.Member, viewer skillViewer, action, search, q string) (views.SkillsPart, error) {
	ctx := r.Context()
	marks, err := p.st.MemberMarks(ctx, m.ID)
	if err != nil {
		return views.SkillsPart{}, err
	}
	part := views.SkillsPart{Whose: m.Name + "'s", Action: action, Search: search, Query: q}
	if viewer.gymnast {
		part.Whose = "your"
	}
	named, other, err := p.clubSkillsOf(r, m.ClubID)
	if err != nil {
		return views.SkillsPart{}, err
	}
	clubItems := map[string]store.ClubSkill{}
	for _, c := range other {
		clubItems[c.Item()] = c
	}
	t := trackingOf(marks, clubItems)
	items, byItem := t.items, t.byItem
	side := func(k store.Mark, ok bool) *views.SideMark {
		if !ok {
			return nil
		}
		v := &views.SideMark{State: k.State, By: k.ByName, At: k.At.In(local).Format("2 Jan")}
		if (viewer.gymnast && k.Side == store.SideGymnast) || (viewer.coach != nil && k.ByCoach == viewer.coach.ID) {
			v.By = "You"
		}
		return v
	}
	// fill gives a row both sides' marks, the viewer's own, and for a coach
	// the gymnast's to confirm.
	fill := func(v *views.MarkView, sides map[string]store.Mark) {
		g, hasG := sides[store.SideGymnast]
		c, hasC := sides[store.SideCoach]
		v.Gymnast, v.Coach = side(g, hasG), side(c, hasC)
		switch {
		case viewer.gymnast && hasG:
			v.Mine = g.State
		case viewer.coach != nil && hasC:
			v.Mine = c.State
		}
		if viewer.coach != nil && hasG && (!hasC || c.State != g.State) {
			v.Confirm = g.State
		}
	}
	// view is a skill as a row, with its marks.
	view := func(s skills.TrampolineSkill, data string, club *store.ClubSkill, sides map[string]store.Mark) views.MarkView {
		v := views.MarkView{Skill: data, Name: skillNameBeside(s), FIG: s.FIGNotation()}
		switch {
		case club != nil:
			v = views.MarkView{Club: club.ID, Name: club.Name, FIG: club.Note}
		case clubName(named, s) != "":
			v.Name, v.FIG = clubName(named, s), skillWithNotation(s)
		}
		fill(&v, sides)
		return v
	}
	can, learning, differ := views.SkillGroup{Title: "Can do"}, views.SkillGroup{Title: "Learning"}, views.SkillGroup{Title: "Marked differently"}
	for _, it := range items {
		v := view(it.skill, it.data, it.club, it.sides)
		if v.Confirm != "" {
			part.ToConfirm++
		}
		states := map[string]bool{}
		for _, k := range it.sides {
			states[k.State] = true
		}
		switch {
		case len(states) > 1:
			differ.Skills = append(differ.Skills, v)
		case states[store.CanDo]:
			can.Skills = append(can.Skills, v)
		default:
			learning.Skills = append(learning.Skills, v)
		}
	}
	part.Marked = []views.SkillGroup{can, learning}
	if len(differ.Skills) > 0 {
		part.Marked = append(part.Marked, differ)
	}
	kept, err := p.st.MemberRoutines(ctx, m.ID)
	if err != nil {
		return views.SkillsPart{}, err
	}
	levels, err := p.st.ClubLevels(ctx, m.ClubID)
	if err != nil {
		return views.SkillsPart{}, err
	}
	gymnastName := m.Name
	if viewer.gymnast {
		gymnastName = "You"
	}
	part.Levels = levelsProgress(levels, clubItems, t, kept, gymnastName, fill)
	if action == "" {
		return part, nil
	}
	// choice is a skill to mark, with its marks if it has any (found by
	// TrampolineSkill.Equal, the one rule for the same skill).
	choice := func(s skills.TrampolineSkill) views.MarkView {
		s.SetTariff()
		s.Name = skills.FindCommonSkillName(s)
		data, _ := json.Marshal(s)
		var sides map[string]store.Mark
		for _, it := range items {
			if it.club == nil && it.skill.Equal(&s) {
				sides = it.sides
				break
			}
		}
		v := view(s, string(data), nil, sides)
		v.Tariff = fmt.Sprintf("%.1f", s.Tariff)
		return v
	}
	// clubChoice is one of the club's own skills to track, with its marks.
	clubChoice := func(c store.ClubSkill) views.MarkView {
		var sides map[string]store.Mark
		if it := byItem[c.Item()]; it != nil {
			sides = it.sides
		}
		return view(skills.TrampolineSkill{}, "", &c, sides)
	}
	ours := views.SkillGroup{Title: "Your club's skills"}
	for _, s := range named {
		ours.Skills = append(ours.Skills, choice(s))
		if q != "" && strings.Contains(strings.ToLower(s.CustomName), strings.ToLower(q)) {
			part.Results = append(part.Results, choice(s))
		}
	}
	for _, c := range other {
		ours.Skills = append(ours.Skills, clubChoice(c))
		if q != "" && strings.Contains(strings.ToLower(c.Name), strings.ToLower(q)) {
			part.Results = append(part.Results, clubChoice(c))
		}
	}
	for _, res := range catalog.Search(q) {
		part.Results = append(part.Results, choice(res.Skill))
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
	if len(ours.Skills) > 0 {
		part.Browse = append(part.Browse, ours)
	}
	for _, c := range catalog.Categories() {
		g := views.SkillGroup{Title: c.Label}
		// A body-landing tile is each of the catalogue's skills it gives,
		// after one landing or another.
		for _, group := range c.Groups {
			for _, e := range group.Options {
				for _, s := range e.Versions() {
					if catalog.InCatalogue(s) {
						g.Skills = append(g.Skills, choice(s))
					}
				}
			}
		}
		for _, e := range c.Entries {
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

// clubSkillsPath is the coaches' page for the club's own skills.
func clubSkillsPath(token string) string { return coachPath(token) + "/club-skills" }

// clubSkillsPage is the coaches' page for the club's own skills, with a
// search of the catalog for a skill to name (q).
func (p *competitionPages) clubSkillsPage(w http.ResponseWriter, r *http.Request) {
	coach, club, ok := p.coach(w, r)
	if !ok {
		return
	}
	p.renderClubSkills(w, r, coach, club, nil)
}

// renderClubSkills shows a coach's Club skills, asking before a change (ask).
func (p *competitionPages) renderClubSkills(w http.ResponseWriter, r *http.Request, coach store.Coach, club string, ask *views.Ask) {
	list, err := p.st.ClubSkills(r.Context(), coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	token, q := r.PathValue("token"), r.URL.Query().Get("q")
	page := views.ClubSkillsPage{Frame: coachFrame(r, coach, club, coachClubSkillsTab, ask), Action: clubSkillsPath(token), Search: clubSkillsPath(token), Query: q, Max: store.MaxClubSkills}
	var named []skills.TrampolineSkill
	for _, c := range list {
		v := views.ClubSkillView{Name: c.Name, What: "To track", Note: c.Note, By: c.ByName, Remove: clubSkillsPath(token) + "/" + c.ID + "/remove"}
		var s skills.TrampolineSkill
		if c.Skill != "" && json.Unmarshal([]byte(c.Skill), &s) == nil {
			v.What = skillWithNotation(s)
			named = append(named, s)
		}
		page.Skills = append(page.Skills, v)
	}
	for _, res := range catalog.Search(q) {
		s := res.Skill
		data, _ := json.Marshal(s)
		page.Results = append(page.Results, views.CatalogChoice{Skill: string(data), Name: skillNameBeside(s), FIG: s.FIGNotation(), ClubName: clubName(named, s)})
	}
	render(w, r, views.CoachClubSkills(page))
}

// addClubSkill is a coach adding one of the club's own skills: a name for a
// catalog skill (skill: its JSON), or something else to track (name, note).
func (p *competitionPages) addClubSkill(w http.ResponseWriter, r *http.Request) {
	coach, _, ok := p.coach(w, r)
	if !ok {
		return
	}
	skill := ""
	if raw := r.FormValue("skill"); raw != "" {
		routine, err := parseRoutineJSON("[" + raw + "]")
		if err != nil || len(routine) != 1 {
			badRequest(w, errors.New("that skill couldn't be read"))
			return
		}
		routine[0].Name = skills.FindCommonSkillName(routine[0])
		data, err := json.Marshal(routine[0])
		if err != nil {
			badRequest(w, err)
			return
		}
		skill = string(data)
	}
	name := strings.TrimSpace(r.FormValue("name"))
	notice := "Added " + name + "."
	switch err := p.st.AddClubSkill(r.Context(), coach, name, r.FormValue("note"), skill); {
	case errors.Is(err, store.ErrSameName):
		notice = "The club already has a skill called " + name + ": choose another name."
	case errors.Is(err, store.ErrLimit):
		notice = fmt.Sprintf("The club has %d skills of its own, the most there's room for: remove one first.", store.MaxClubSkills)
	case err != nil && name == "":
		notice = "A club skill needs a name."
	case err != nil:
		failed(w, r, err)
		return
	}
	seeOther(w, r, clubSkillsPath(r.PathValue("token")), notice)
}

// removeClubSkill is a coach removing one of the club's own skills.
func (p *competitionPages) removeClubSkill(w http.ResponseWriter, r *http.Request) {
	coach, club, ok := p.coach(w, r)
	if !ok {
		return
	}
	list, err := p.st.ClubSkills(r.Context(), coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	i := slices.IndexFunc(list, func(c store.ClubSkill) bool { return c.ID == r.PathValue("id") })
	if i < 0 {
		failed(w, r, store.ErrNotFound)
		return
	}
	if !asked(r) {
		text := "Every member's and coach's marks on it go with it."
		if list[i].Skill != "" {
			text = "It's your club's name for a skill: the skill's own name shows again, and marks on the skill stay."
		}
		p.renderClubSkills(w, r, coach, club, inPlace(r, views.Ask{
			Remove: true, // takes something away: red
			Title:  "Remove " + list[i].Name + "?", Text: text,
			Button: "Remove " + list[i].Name, Back: clubSkillsPath(r.PathValue("token")),
		}))
		return
	}
	if err := p.st.RemoveClubSkill(r.Context(), coach, r.PathValue("id")); err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, clubSkillsPath(r.PathValue("token")), "Removed.")
}

// marked is a skill's marks, both sides: a catalog skill's, or one of the
// club's own to track.
type marked struct {
	skill skills.TrampolineSkill
	data  string
	club  *store.ClubSkill
	sides map[string]store.Mark
}

// tracking is a member's marks sorted for tracking them.
type tracking struct {
	// items are the skills marked, a catalog skill's or one of the club's own
	// to track, each with both sides' marks, in the order marks come (newest
	// first); byItem has them by item.
	items  []*marked
	byItem map[string]*marked
	// levelMarks are the club levels' items marked as a whole, both sides,
	// by item.
	levelMarks map[string]map[string]store.Mark
	// sideSkills are the catalog skills each side marked, by side and state.
	sideSkills map[string]map[string][]skills.TrampolineSkill
}

// trackingOf sorts a member's marks (newest first, as MemberMarks gives
// them), with the club's own skills to track by item (clubItems).
func trackingOf(marks []store.Mark, clubItems map[string]store.ClubSkill) tracking {
	t := tracking{byItem: map[string]*marked{}, levelMarks: map[string]map[string]store.Mark{},
		sideSkills: map[string]map[string][]skills.TrampolineSkill{store.SideGymnast: {}, store.SideCoach: {}}}
	for _, k := range marks {
		if strings.HasPrefix(k.Item, levelItemPrefix) {
			if t.levelMarks[k.Item] == nil {
				t.levelMarks[k.Item] = map[string]store.Mark{}
			}
			t.levelMarks[k.Item][k.Side] = k
			continue
		}
		it := t.byItem[k.Item]
		if it == nil {
			it = &marked{data: k.Skill, sides: map[string]store.Mark{}}
			if c, ok := clubItems[k.Item]; ok {
				it.club = &c
			} else if json.Unmarshal([]byte(k.Skill), &it.skill) != nil {
				continue
			}
			t.byItem[k.Item] = it
			t.items = append(t.items, it)
		}
		it.sides[k.Side] = k
	}
	for _, it := range t.items {
		if it.club != nil {
			continue
		}
		for s, k := range it.sides {
			if t.sideSkills[s] != nil {
				t.sideSkills[s][k.State] = append(t.sideSkills[s][k.State], it.skill)
			}
		}
	}
	return t
}

// sides is an item's marks, both sides ("" for none).
func (t tracking) sides(item string) map[string]store.Mark {
	if it, ok := t.byItem[item]; ok {
		return it.sides
	}
	return nil
}
