package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Officials (ADR 0005 step 3): what gymnasts offer to judge and help with,
// the people the organiser adds, the panels each discipline needs and who may
// judge what. The officials rota (step 5) puts them on panels.

func (p *competitionPages) registerOfficials(handle func(string, http.HandlerFunc)) {
	handle("GET /competitions/admin/{token}/officials", p.officials)
	handle("POST /competitions/admin/{token}/officials/settings", p.officialSettings)
	handle("POST /competitions/admin/{token}/officials/add", p.addOfficial)
	handle("POST /competitions/admin/{token}/officials/{id}/qualified", p.markQualified)
	handle("POST /competitions/admin/{token}/officials/{id}/remove", p.removeOfficial)
	handle("POST /clubs/member/{token}/competitions/{id}/offer", p.saveOwnOffer)
	handle("GET /clubs/admin/{token}/members/{member}/competitions/{id}/offer", p.editMemberOffer)
	handle("POST /clubs/admin/{token}/members/{member}/competitions/{id}/offer", p.saveMemberOfferForMember)
	handle("POST /competitions/entry/{token}/offer", p.saveIndividualOffer)
}

// offerKey is a discipline's name in the offer form.
func offerKey(d string) string {
	if d == competitions.Trampoline {
		return "trampoline"
	}
	return d
}

// offerForm is an offer as the form shows it, for the competition's disciplines.
func offerForm(c competitions.Competition, o competitions.Offer, action string) views.OfferForm {
	f := views.OfferForm{Action: action, Recorder: o.Recorder, Marshal: o.Marshal, Summary: o.Describe()}
	for _, d := range c.Disciplines() {
		j, judges := o.Judge[d]
		f.Disciplines = append(f.Disciplines, views.OfferDiscipline{
			Key: offerKey(d), Name: competitions.DisciplineName(d), Levels: c.LevelNames(d),
			Judge: judges, UpTo: j.UpTo, Chair: j.Chair,
		})
	}
	return f
}

// postedOffer reads an offer: for each discipline, judge-<key>, upto-<key> and
// chair-<key>; and recorder and marshal.
func postedOffer(r *http.Request, c competitions.Competition) competitions.Offer {
	o := competitions.Offer{Recorder: r.FormValue("recorder") == "1", Marshal: r.FormValue("marshal") == "1"}
	for _, d := range c.Disciplines() {
		key := offerKey(d)
		if r.FormValue("judge-"+key) != "1" {
			continue
		}
		if o.Judge == nil {
			o.Judge = map[string]competitions.JudgeOffer{}
		}
		o.Judge[d] = competitions.JudgeOffer{UpTo: r.FormValue("upto-" + key), Chair: r.FormValue("chair-"+key) == "1"}
	}
	return o
}

// --- Members, comp secs and individuals ---

func (p *competitionPages) saveOwnOffer(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	c, ok := p.memberCompetition(w, r, m)
	if !ok {
		return
	}
	o := postedOffer(r, c.Competition)
	if err := c.ValidateOffer(o); err != nil {
		badRequest(w, err)
		return
	}
	if err := p.st.SaveMemberOffer(r.Context(), m.ID, c.ID, o); err != nil {
		failed(w, r, err)
		return
	}
	notice := "Saved what you offer at " + c.Name + ". Your club sends it with its entries."
	http.Redirect(w, r, memberPath(r.PathValue("token"))+"?notice="+urlQuery(notice), http.StatusSeeOther)
}

func (p *competitionPages) editMemberOffer(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	m, c, ok := p.memberOfClub(w, r, club)
	if !ok {
		return
	}
	o, err := p.st.MemberOffer(r.Context(), m.ID, c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	render(w, r, views.ClubOfferEditPage(views.ClubOfferEdit{
		Base: clubPath(r.PathValue("token")), Club: club.Name, Member: m.Name, Competition: summary(c.Competition, p.now()),
		Form: offerForm(c.Competition, o, r.URL.Path),
	}))
}

func (p *competitionPages) saveMemberOfferForMember(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	m, c, ok := p.memberOfClub(w, r, club)
	if !ok {
		return
	}
	o := postedOffer(r, c.Competition)
	if err := c.ValidateOffer(o); err != nil {
		badRequest(w, err)
		return
	}
	if err := p.st.SaveMemberOffer(r.Context(), m.ID, c.ID, o); err != nil {
		failed(w, r, err)
		return
	}
	notice := fmt.Sprintf("Saved what %s offers at %s. Send to pass it on.", m.Name, c.Name)
	http.Redirect(w, r, clubPath(r.PathValue("token"))+"?notice="+urlQuery(notice), http.StatusSeeOther)
}

func (p *competitionPages) saveIndividualOffer(w http.ResponseWriter, r *http.Request) {
	_, c, ok := p.own(w, r)
	if !ok {
		return
	}
	o := postedOffer(r, c.Competition)
	if err := c.ValidateOffer(o); err != nil {
		badRequest(w, err)
		return
	}
	if err := p.st.SetIndividualOffer(r.Context(), r.PathValue("token"), o); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, "/competitions/entry/"+r.PathValue("token"), http.StatusSeeOther)
}

// --- The organiser ---

func officialsPath(r *http.Request) string { return adminPath(r.PathValue("token")) + "/officials" }

func (p *competitionPages) officials(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	people, err := p.st.Officials(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	page := views.OfficialsPage{
		Base: adminPath(r.PathValue("token")), Competition: summary(c.Competition, p.now()), Notice: r.URL.Query().Get("notice"),
		Judge: c.Officials.Judge, RecordersChange: c.Officials.RecordersChange, MarshalsChange: c.Officials.MarshalsChange, Add: offerForm(c.Competition, competitions.Offer{}, ""),
	}
	for _, d := range c.Disciplines() {
		panel := c.Officials.Panel(d)
		page.Panels = append(page.Panels, views.PanelSetting{
			Key: offerKey(d), Name: competitions.DisciplineName(d), Chair: panel.Chair, Execution: panel.Execution,
			Difficulty: panel.Difficulty, HD: panel.HD, HasHD: d == competitions.Trampoline || d == competitions.Synchro, Sync: panel.Sync, HasSync: d == competitions.Synchro,
			Recorder: panel.Recorder, Marshal: panel.Marshal,
		})
		cover := views.Cover{Name: competitions.DisciplineName(d), PanelJudges: panel.Judges(), PanelChairs: panel.Chair}
		for _, o := range people {
			j, judges := o.Offer.Judge[d]
			if !judges || (c.Officials.Judge == competitions.JudgeQualified && !o.Qualified) {
				continue
			}
			cover.Judges++
			if j.Chair {
				cover.Chairs++
			}
		}
		page.Cover = append(page.Cover, cover)
	}
	for _, o := range people {
		row := views.OfficialRow{ID: o.ID, Name: o.Name, Club: o.ClubName, Offer: o.Offer.Describe(), Qualified: o.Qualified, Added: o.Added}
		switch {
		case o.Added:
			row.Source = "added by you"
		case o.EntryID != "":
			row.Source = "entering on their own"
		default:
			row.Source = "sent by their club"
		}
		page.Officials = append(page.Officials, row)
	}
	render(w, r, views.CompetitionOfficials(page))
}

// officialSettings saves each discipline's panel (panel-<key>-<role>) and who
// may judge what (judge).
func (p *competitionPages) officialSettings(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	settings := competitions.OfficialSettings{Judge: r.FormValue("judge"), Panels: map[string]competitions.Panel{}, RecordersChange: r.FormValue("recordersChange") == "1", MarshalsChange: r.FormValue("marshalsChange") == "1"}
	for _, d := range c.Disciplines() {
		n := func(role string) int {
			v, err := strconv.Atoi(strings.TrimSpace(r.FormValue("panel-" + offerKey(d) + "-" + role)))
			if err != nil {
				return -1
			}
			return v
		}
		panel := competitions.Panel{Chair: n("chair"), Execution: n("execution"), Difficulty: n("difficulty"), Recorder: n("recorder"), Marshal: n("marshal")}
		if (d == competitions.Trampoline || d == competitions.Synchro) && r.FormValue("panel-"+offerKey(d)+"-hd") != "" {
			panel.HD = n("hd")
		}
		if d == competitions.Synchro && r.FormValue("panel-synchro-sync") != "" {
			panel.Sync = n("sync")
		}
		if panel != competitions.DefaultPanel(d) {
			settings.Panels[d] = panel
		}
	}
	notice := "Panels and judging saved."
	if err := settings.Check(); err != nil {
		notice = "Nothing was changed: " + strings.Join(sentences(err), " ")
	} else if err := p.st.SetOfficialSettings(r.Context(), c.ID, settings); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, officialsPath(r)+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

func (p *competitionPages) addOfficial(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	name, club := strings.TrimSpace(r.FormValue("name")), strings.TrimSpace(r.FormValue("club"))
	o := postedOffer(r, c.Competition)
	var problems []string
	if err := competitions.CheckName("official", name); err != nil {
		problems = append(problems, sentences(err)...)
	}
	if club != "" {
		if err := competitions.CheckName("club", club); err != nil {
			problems = append(problems, sentences(err)...)
		}
	}
	if err := c.ValidateOffer(o); err != nil {
		problems = append(problems, sentences(err)...)
	}
	if o.Empty() {
		problems = append(problems, "Tick what they can judge or help with.")
	}
	notice := "Added " + name + "."
	if len(problems) > 0 {
		notice = "Nobody was added: " + strings.Join(problems, " ")
	} else if _, err := p.st.AddOfficial(r.Context(), c.ID, name, club, o); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, officialsPath(r)+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

func (p *competitionPages) markQualified(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := p.st.SetQualified(r.Context(), c.ID, r.PathValue("id"), r.FormValue("on") == "1"); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, officialsPath(r), http.StatusSeeOther)
}

func (p *competitionPages) removeOfficial(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := p.st.RemoveOfficial(r.Context(), c.ID, r.PathValue("id")); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, officialsPath(r)+"?notice="+url.QueryEscape("Removed."), http.StatusSeeOther)
}

// clubOffers are what a club's members offer at a competition, as the club
// page lists them.
func (p *competitionPages) clubOffers(r *http.Request, base string, club store.Club, c store.Competition, members []store.Member) ([]views.ClubOfferRow, error) {
	offers, err := p.st.ClubOffers(r.Context(), club.ID, c.ID)
	if err != nil {
		return nil, err
	}
	var out []views.ClubOfferRow
	for _, m := range members {
		o, ok := offers[m.ID]
		if !ok {
			continue
		}
		row := views.ClubOfferRow{Member: m.Name, Offer: o.Describe()}
		if c.Open(p.now()) {
			row.Edit = base + "/members/" + m.ID + "/competitions/" + c.ID + "/offer"
		}
		out = append(out, row)
	}
	return out, nil
}
