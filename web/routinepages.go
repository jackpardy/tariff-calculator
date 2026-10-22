package web

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/requirements"
	"tariffCalculator/skills"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// A member's routines, kept on their page (ADR 0011 Decision 1): the member's
// Routines tab, where the routine builder's Save to my page posts; a coach's
// Gymnasts, with each gymnast's routines, a note on each, the skills in each
// no coach has marked Can do, checking one against a level the coach chooses,
// and adding routines to the gymnast's page; and the comp sec's view of a
// member's routines from Members.

// maxKeepBytes bounds a request keeping routines: a browser's worth, each
// with its requirements.
const maxKeepBytes = 512 << 10

// maxKeptSet is the most a kept routine's own requirements may be, as JSON.
const maxKeptSet = 16 << 10

func (p *competitionPages) registerRoutines(handle func(string, http.HandlerFunc)) {
	handle("GET /clubs/member/{token}/routines", p.memberRoutines)
	handle("POST /clubs/member/{token}/routines", p.keepRoutines)
	handle("POST /clubs/member/{token}/routines/{routine}/rename", p.renameRoutine)
	handle("POST /clubs/member/{token}/routines/{routine}/delete", p.deleteRoutine)
	handle("GET /clubs/coach/{token}/gymnasts", p.coachGymnasts)
	handle("GET /clubs/coach/{token}/gymnasts/{member}", p.coachGymnast)
	handle("POST /clubs/coach/{token}/gymnasts/{member}/routines", p.addRoutinesAsCoach)
	handle("POST /clubs/coach/{token}/gymnasts/{member}/routines/{routine}/note", p.noteRoutine)
	handle("GET /clubs/admin/{token}/members/{member}/routines", p.clubMemberRoutines)
}

// keepsRoutines says a request keeps routines, for its larger body: a
// member's, or a coach's for a gymnast.
func keepsRoutines(path string) bool {
	return (strings.HasPrefix(path, "/clubs/member/") || strings.HasPrefix(path, "/clubs/coach/")) && strings.HasSuffix(path, "/routines")
}

// memberRoutinesPath is a member's Routines tab.
func memberRoutinesPath(token string) string { return memberPath(token) + "/routines" }

// coachGymnastsPath is a coach's Gymnasts.
func coachGymnastsPath(token string) string { return coachPath(token) + "/gymnasts" }

// memberRoutines is a member's Routines tab.
func (p *competitionPages) memberRoutines(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	p.renderMemberRoutines(w, r, m, nil)
}

// renderMemberRoutines shows a member's Routines tab, asking before a change
// (ask).
func (p *competitionPages) renderMemberRoutines(w http.ResponseWriter, r *http.Request, m store.Member, ask *views.Ask) {
	page, _, ok := p.memberPage(w, r, m, memberRoutinesTab)
	if !ok {
		return
	}
	list, err := p.st.MemberRoutines(r.Context(), m.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	named, _, err := p.clubSkillsOf(r, m.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	base := memberRoutinesPath(r.PathValue("token"))
	page.Routines = routinesPart(list, named, "You haven't kept any routines here yet.", func(v *views.RoutineView) {
		v.Rename, v.Delete = base+"/"+v.ID+"/rename", base+"/"+v.ID+"/delete"
	})
	page.Routines.Add = base
	page.Ask = ask
	render(w, r, views.MemberRoutines(page))
}

// keptRoutine is a routine posted to keep, as the browser saves it: its id
// there, its name, skills, requirements (a built-in reference or the set's
// JSON, as SetStore.payload gives) and checks.
type keptRoutine struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Skills       json.RawMessage `json:"skills"`
	Requirements string          `json:"requirements"`
	Checks       json.RawMessage `json:"checks"`
}

// keepRoutines keeps the posted routines (routines: a JSON list of
// keptRoutine) on a member's page, from the routine builder or the Routines
// tab, and goes back to the tab saying how many.
func (p *competitionPages) keepRoutines(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	in, unread, err := postedRoutines(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	saved, full, err := p.st.SaveRoutines(r.Context(), m.ClubID, m.ID, in)
	if err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, memberRoutinesPath(r.PathValue("token")), keptNotice(saved, full, unread, "your page"))
}

// addRoutinesAsCoach adds the posted routines (as keepRoutines) to the page
// of a gymnast the coach sees, and goes back to the gymnast.
func (p *competitionPages) addRoutinesAsCoach(w http.ResponseWriter, r *http.Request) {
	coach, _, ok := p.coach(w, r)
	if !ok {
		return
	}
	m, err := p.st.CoachMember(r.Context(), coach, r.PathValue("member"))
	if err != nil {
		failed(w, r, err)
		return
	}
	in, unread, err := postedRoutines(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	saved, full, err := p.st.AddRoutinesAsCoach(r.Context(), coach, m.ID, in)
	if err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, coachGymnastsPath(r.PathValue("token"))+"/"+m.ID, keptNotice(saved, full, unread, m.Name+"'s page"))
}

// postedRoutines are the routines posted to keep (routines: a JSON list of
// keptRoutine), leaving out empty ones, with how many couldn't be read.
func postedRoutines(r *http.Request) (in []store.RoutineIn, unread int, err error) {
	var posted []keptRoutine
	if err := json.Unmarshal([]byte(r.FormValue("routines")), &posted); err != nil {
		return nil, 0, fmt.Errorf("the routines couldn't be read")
	}
	for _, k := range posted {
		var skills []json.RawMessage
		if json.Unmarshal(k.Skills, &skills) == nil && len(skills) == 0 {
			continue // an empty routine: nothing to keep
		}
		kept, ok := routineToKeep(k)
		if !ok {
			unread++
			continue
		}
		in = append(in, kept)
	}
	return in, unread, nil
}

// keptNotice says how many routines were kept on a page (where), how many
// didn't fit, and how many couldn't be read.
func keptNotice(saved, full, unread int, where string) string {
	notice := "Nothing to keep: choose a routine with skills."
	if saved > 0 {
		notice = fmt.Sprintf("Kept %s on %s.", plural(saved, "routine", "routines"), where)
	}
	if full > 0 {
		notice += fmt.Sprintf(" %s didn't fit: there's room for %d, so delete one to keep another.", plural(full, "routine", "routines"), store.MaxRoutines)
	}
	if unread > 0 {
		notice += fmt.Sprintf(" %s couldn't be read.", plural(unread, "routine", "routines"))
	}
	return notice
}

// routineToKeep is a posted routine as it's kept: its skills read and checked
// as the builder's are, its requirements kept only if they can be used, and
// its checks only if they're a small JSON object. An empty routine, or one
// that can't be read, isn't kept.
func routineToKeep(k keptRoutine) (store.RoutineIn, bool) {
	routine, err := parseRoutineJSON(string(k.Skills))
	if err != nil || len(routine) == 0 || len(routine) > store.MaxRoutineSkills {
		return store.RoutineIn{}, false
	}
	data, err := json.Marshal(routine)
	if err != nil {
		return store.RoutineIn{}, false
	}
	out := store.RoutineIn{Name: k.Name, Skills: string(data)}
	if len(k.ID) <= 40 {
		out.Source = k.ID
	}
	if set := strings.TrimSpace(k.Requirements); set != "" && len(set) <= maxKeptSet {
		if _, err := requirements.ResolveSet(set); err == nil {
			out.Requirements = set
		}
	}
	var checks map[string]any
	if len(k.Checks) <= 1<<10 && json.Unmarshal(k.Checks, &checks) == nil && checks != nil {
		out.Checks = string(k.Checks)
	}
	return out, true
}

// renameRoutine renames one of a member's routines.
func (p *competitionPages) renameRoutine(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	notice := "Renamed."
	if err := p.st.RenameRoutine(r.Context(), m.ID, r.PathValue("routine"), r.FormValue("name")); errors.Is(err, store.ErrNotFound) {
		failed(w, r, err)
		return
	} else if err != nil {
		notice = "A routine needs a name."
	}
	seeOther(w, r, memberRoutinesPath(r.PathValue("token")), notice)
}

// deleteRoutine deletes one of a member's routines from their page.
func (p *competitionPages) deleteRoutine(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	if !asked(r) {
		list, err := p.st.MemberRoutines(r.Context(), m.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		i := slices.IndexFunc(list, func(k store.Routine) bool { return k.ID == r.PathValue("routine") })
		if i < 0 {
			failed(w, r, store.ErrNotFound)
			return
		}
		p.renderMemberRoutines(w, r, m, inPlace(r, views.Ask{
			Remove: true, // takes something away: red
			Title:  "Delete " + list[i].Name + " from your page?",
			Text:   "Your coaches and comp sec stop seeing it, with any coach's note on it. A copy saved in this browser stays, to add again.",
			Button: "Delete it", Back: memberRoutinesPath(r.PathValue("token")),
		}))
		return
	}
	if err := p.st.DeleteRoutine(r.Context(), m.ID, r.PathValue("routine")); err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, memberRoutinesPath(r.PathValue("token")), "Deleted from your page.")
}

// coachFrame is the top of a coach's page, with the section shown current,
// asking before a change (ask) if there's one to ask about.
func coachFrame(r *http.Request, coach store.Coach, club, section string, ask *views.Ask) views.CoachFrame {
	return views.CoachFrame{Club: club, Coach: coach.Name, Notice: r.URL.Query().Get("notice"), Ask: ask, Menu: coachMenu(r.PathValue("token"), section)}
}

// coachGymnasts lists the gymnasts a coach sees, with how many routines each keeps.
func (p *competitionPages) coachGymnasts(w http.ResponseWriter, r *http.Request) {
	coach, club, ok := p.coach(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	members, err := p.st.CoachMembers(ctx, coach)
	if err != nil {
		failed(w, r, err)
		return
	}
	// By name as people read it: Róisín before Ruairí.
	slices.SortStableFunc(members, func(a, b store.Member) int { return competitions.CompareNames(a.Name, b.Name) })
	counts, err := p.st.RoutineCounts(ctx, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	can, toConfirm, err := p.st.MarkCounts(ctx, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	seesAll, err := p.st.CoachesSeeAll(ctx, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	levels, err := p.st.ClubLevels(ctx, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	standing, err := p.levelStandings(r, coach.ClubID, levels)
	if err != nil {
		failed(w, r, err)
		return
	}
	base := coachGymnastsPath(r.PathValue("token"))
	page := views.CoachGymnastsPage{Frame: coachFrame(r, coach, club, coachGymnastsTab, nil), SeesAll: seesAll}
	for _, m := range members {
		page.Gymnasts = append(page.Gymnasts, views.CoachGymnast{Name: m.Name, Routines: counts[m.ID], Can: can[m.ID], ToConfirm: toConfirm[m.ID], Open: base + "/" + m.ID,
			List: views.ListRow{Name: m.Name, Values: map[string][]string{"routines": {hasValue(counts[m.ID] > 0)}, "mine": {hasValue(m.CoachID == coach.ID)},
				"confirm": {hasValue(toConfirm[m.ID] > 0)}, "level": standing[m.ID]}}})
	}
	page.Tools = views.ListTools{ID: "gymnasts", Noun: "gymnasts", Total: len(members), Filters: []views.ListFilter{
		hasFilter("routines", "Routines", "Routines or not", "Has routines", "No routines"),
		hasFilter("mine", "Coach", "Any coach", "Chose you", "Didn't choose you"),
		hasFilter("confirm", "Skills", "Any skills", "Marks to confirm", "None to confirm"),
	}}
	if len(levels) > 0 {
		page.Tools.Filters = append(page.Tools.Filters, levelFilter(levels))
	}
	render(w, r, views.CoachGymnasts(page))
}

// coachGymnast is one gymnast a coach sees, with their routines: to note,
// to see which skills no coach has marked Can do, and to check against a
// level the coach chooses (routine, against: shown checked against it).
func (p *competitionPages) coachGymnast(w http.ResponseWriter, r *http.Request) {
	coach, club, ok := p.coach(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	m, err := p.st.CoachMember(ctx, coach, r.PathValue("member"))
	if err != nil {
		failed(w, r, err)
		return
	}
	list, err := p.st.MemberRoutines(ctx, m.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	named, other, err := p.clubSkillsOf(r, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	marks, err := p.st.MemberMarks(ctx, m.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	levels, err := p.st.ClubLevels(ctx, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	clubItems := map[string]store.ClubSkill{}
	for _, c := range other {
		clubItems[c.Item()] = c
	}
	can := trackingOf(marks, clubItems).sideSkills[store.SideCoach][store.CanDo]
	notCan, notCanJSON := map[string][]string{}, map[string][]string{}
	for _, kept := range list {
		notCan[kept.ID], notCanJSON[kept.ID] = notMarkedCan(kept, can, named)
	}
	choices, against := checkChoices(levels), map[string]string{}
	for _, l := range levels {
		against["club:"+l.ID] = l.Set
	}
	routine, chosen := r.URL.Query().Get("routine"), r.URL.Query().Get("against")
	if _, ok := requirements.LookupBuiltin(chosen); ok && strings.HasPrefix(chosen, requirements.BuiltinPrefix) {
		against[chosen] = chosen
	}
	if against[chosen] == "" {
		chosen = ""
	}
	base := coachGymnastsPath(r.PathValue("token"))
	page := views.CoachGymnastsPage{Frame: coachFrame(r, coach, club, coachGymnastsTab, nil), Gymnast: m.Name, Back: base, Skills: base + "/" + m.ID + "/skills"}
	page.Routines = routinesPart(list, named, m.Name+" hasn't kept any routines on their page yet.", func(v *views.RoutineView) {
		v.NoteAction = base + "/" + m.ID + "/routines/" + v.ID + "/note"
		v.NotCan, v.SkillsLink = notCan[v.ID], page.Skills
		v.MarkCan, v.NotCanSkills = page.Skills, notCanJSON[v.ID]
		v.Check = &views.RoutineCheck{Action: base + "/" + m.ID, Choices: choices}
	})
	if chosen != "" {
		for i, kept := range list {
			if kept.ID != routine {
				continue
			}
			v := page.Routines.Routines[i]
			kept.Requirements = against[chosen]
			checked := routineView(kept, named)
			v.Card, v.Difficulty, v.Open = checked.Card, checked.Difficulty, checked.Open
			v.Check.Chosen = chosen
			page.Routines.Routines[i] = v
		}
	}
	page.Routines.Add = base + "/" + m.ID + "/routines"
	entries, err := p.st.MemberEntries(ctx, m.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	comps := map[string]store.Competition{}
	for _, e := range entries {
		c, ok := comps[e.CompetitionID]
		if !ok {
			if c, err = p.st.Competition(ctx, e.CompetitionID); err != nil {
				failed(w, r, err)
				return
			}
			comps[c.ID] = c
		}
		page.Entries = append(page.Entries, views.CoachGymnastEntry{Competition: c.Name, Event: e.Entry.Event(), Signoff: memberSignoff(c.Competition, e),
			Open: coachPath(r.PathValue("token")) + "/members/" + m.ID + "/competitions/" + c.ID + "?discipline=" + e.Discipline})
	}
	render(w, r, views.CoachGymnastPage(page))
}

// notMarkedCan are the skills of a kept routine that aren't among those a
// coach has marked Can do (can), each once, by the club's name where it has
// one (named), and as JSON, to mark.
func notMarkedCan(kept store.Routine, can, named []skills.TrampolineSkill) (out, asJSON []string) {
	routine, err := parseRoutineJSON(kept.Skills)
	if err != nil {
		return nil, nil
	}
	var seen []skills.TrampolineSkill
	for _, s := range routine {
		same := func(o skills.TrampolineSkill) bool { return o.Equal(&s) }
		if slices.ContainsFunc(can, same) || slices.ContainsFunc(seen, same) {
			continue
		}
		seen = append(seen, s)
		name := clubName(named, s)
		if name == "" {
			name = skillName(s)
		}
		data, _ := json.Marshal(s)
		out, asJSON = append(out, name), append(asJSON, string(data))
	}
	return out, asJSON
}

// checkChoices are the levels a coach can check a routine against: the
// club's levels, then the built-in ones, by group.
func checkChoices(levels []store.ClubLevel) []views.ListOption {
	var out []views.ListOption
	for _, l := range levels {
		out = append(out, views.ListOption{Value: "club:" + l.ID, Label: l.Name, Group: "Your club's levels"})
	}
	for _, g := range requirements.BuiltinGroups() {
		for _, b := range g.Sets {
			out = append(out, views.ListOption{Value: requirements.BuiltinPrefix + b.ID, Label: b.Set.Name, Group: g.Name})
		}
	}
	return out
}

// noteRoutine is a coach's note on a gymnast's routine.
func (p *competitionPages) noteRoutine(w http.ResponseWriter, r *http.Request) {
	coach, _, ok := p.coach(w, r)
	if !ok {
		return
	}
	member, routine := r.PathValue("member"), r.PathValue("routine")
	if err := p.st.NoteRoutine(r.Context(), coach, member, routine, r.FormValue("note")); err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, coachGymnastsPath(r.PathValue("token"))+"/"+member+"#routine-"+routine, "Note saved.")
}

// clubMemberRoutines shows the comp sec a member's routines.
func (p *competitionPages) clubMemberRoutines(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	m, err := p.st.Member(r.Context(), club.ID, r.PathValue("member"))
	if err != nil {
		failed(w, r, err)
		return
	}
	list, err := p.st.MemberRoutines(r.Context(), m.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	named, _, err := p.clubSkillsOf(r, club.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	token := r.PathValue("token")
	page := views.ClubMemberRoutinesPage{
		Club:   views.ClubPage{Base: clubPath(token), Name: club.Name, Menu: clubMenu(r, clubMembers)},
		Member: m.Name, Back: clubMembersPath(token),
		Routines: routinesPart(list, named, m.Name+" hasn't kept any routines on their page yet.", nil),
	}
	render(w, r, views.ClubMemberRoutines(page))
}

// routinesPart is kept routines as a page shows them, with the club's names
// for skills (named), each given its actions by with (nil for none).
func routinesPart(list []store.Routine, named []skills.TrampolineSkill, empty string, with func(*views.RoutineView)) views.RoutinesPart {
	out := views.RoutinesPart{Max: store.MaxRoutines, Empty: empty}
	for _, kept := range list {
		v := routineView(kept, named)
		if with != nil {
			with(&v)
		}
		out.Routines = append(out.Routines, v)
	}
	return out
}

// routineView is a kept routine, checked as the routine builder checks it,
// against its requirements if it has any, with the club's names for its
// skills (named) where the routine hasn't its own.
func routineView(kept store.Routine, named []skills.TrampolineSkill) views.RoutineView {
	v := views.RoutineView{ID: kept.ID, Name: kept.Name, Updated: kept.UpdatedAt.In(local).Format("2 January 2006"),
		Note: kept.Note, NoteBy: kept.NoteBy, NoteBefore: kept.NoteBefore(), AddedBy: kept.AddedName}
	routine, err := routineFrom(postedRoutine{data: kept.Skills, set: kept.Requirements, checks: kept.Checks})
	if err != nil {
		v.Broken = "This routine couldn't be read."
		return v
	}
	if withNames, err := namedRoutine(kept.Skills); err == nil {
		routine.Skills = withNames
	}
	for i, s := range routine.Skills {
		if s.CustomName == "" {
			routine.Skills[i].CustomName = clubName(named, s)
		}
	}
	checked := requirements.Check(routine, nil)
	v.Count = len(routine.Skills)
	v.Card = views.ExerciseCard{Title: kept.Name, Requirements: checked.SetName,
		Validation: checked.Validation, Checks: checked.Checks, Results: checked.Results, Required: checked.Required}
	if checked.SetErr != nil {
		v.Card.SetErr = checked.SetErr.Error()
	}
	for _, res := range checked.Results {
		if !res.Passed {
			v.Unmet++
		}
	}
	if v.Card.Requirements == "" {
		v.Card.Requirements = "Checked against the Code of Points only"
	}
	if checked.Checks.ScoreDifficulty {
		v.Difficulty = fmt.Sprintf("%.1f", checked.Validation.TotalTariff)
	}
	v.Open = builderLink(kept)
	return v
}

// builderLink opens the routine builder with a kept routine to add, as a
// share link (static/js/share.js) does: the routine in the link's fragment,
// as plain JSON, with its own requirements if it has them.
func builderLink(kept store.Routine) string {
	var list []map[string]any
	if err := json.Unmarshal([]byte(kept.Skills), &list); err != nil {
		return ""
	}
	for _, s := range list {
		delete(s, "name") // worked out again, as a share link's are
		delete(s, "tariff")
	}
	routine := map[string]any{"name": kept.Name, "skills": list}
	share := map[string]any{"v": 1, "routines": []any{routine}}
	if strings.HasPrefix(kept.Requirements, requirements.BuiltinPrefix) {
		routine["requirements"] = kept.Requirements
	} else if kept.Requirements != "" {
		routine["requirements"] = "set:0"
		share["sets"] = []json.RawMessage{json.RawMessage(kept.Requirements)}
	}
	if kept.Checks != "" {
		routine["checks"] = json.RawMessage(kept.Checks)
	}
	data, err := json.Marshal(share)
	if err != nil {
		return ""
	}
	return "/#share=j" + base64.RawURLEncoding.EncodeToString(data)
}

// keptForEntry are a member's kept routines, offered for an entry's voluntaries.
func (p *competitionPages) keptForEntry(r *http.Request, memberID string) ([]views.KeptRoutine, error) {
	list, err := p.st.MemberRoutines(r.Context(), memberID)
	if err != nil {
		return nil, err
	}
	var out []views.KeptRoutine
	for _, kept := range list {
		routine, err := parseRoutineJSON(kept.Skills)
		if err != nil || len(routine) == 0 {
			continue
		}
		out = append(out, views.KeptRoutine{Name: kept.Name, Skills: kept.Skills, Count: len(routine)})
	}
	return out, nil
}

// plural is n with its noun, e.g. "1 routine", "3 routines".
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
