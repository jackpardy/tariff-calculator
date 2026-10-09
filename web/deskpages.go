package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Messages from the organisers' desk (roadmap 2026-10-09): the organiser
// sends a message, or asks people to come to the desk, to a club, a flight,
// the officials or a group picked by hand. It is pushed and emailed at once
// to whoever has asked to hear, and shown on each recipient's own page.

func (p *competitionPages) registerDesk(handle func(string, http.HandlerFunc)) {
	handle("GET /competitions/admin/{token}/desk", p.desk)
	handle("POST /competitions/admin/{token}/desk", p.sendDesk)
}

// maxDeskText is the longest message, in characters.
const maxDeskText = 500

// maxDeskShown is how many messages a person's page shows.
const maxDeskShown = 10

// deskPerson is someone a message can be sent to.
type deskPerson struct {
	key, name, detail string
}

// deskPeople are every gymnast and official of a competition, by name.
func (p *competitionPages) deskPeople(r *http.Request, c store.Competition, entries []store.Entry) ([]deskPerson, error) {
	_, names := peopleOf(entries)
	byKey := map[string]*deskPerson{}
	var out []*deskPerson
	add := func(key, name, club, role string) {
		if name == "" {
			return
		}
		d := byKey[key]
		if d == nil {
			d = &deskPerson{key: key, name: name}
			byKey[key] = d
			out = append(out, d)
		}
		for _, part := range []string{club, role} {
			if part != "" && !strings.Contains(d.detail, part) {
				if d.detail != "" {
					d.detail += " · "
				}
				d.detail += part
			}
		}
	}
	clubs := map[string]string{} // person → their club
	keys := personKeys(entries)
	for _, e := range entries {
		if !e.Individual {
			for _, k := range keys[e.ID] {
				clubs[k] = e.ClubName
			}
		}
	}
	for key, name := range names {
		if !strings.HasPrefix(key, "c:") { // not coaches
			add(key, name, clubs[key], "gymnast")
		}
	}
	officials, err := p.rotaOf(r, c, entries)
	if err != nil {
		return nil, err
	}
	for _, o := range officials {
		add(o.Key, o.Name, o.Club, "official")
	}
	slices.SortStableFunc(out, func(a, b *deskPerson) int {
		if x := strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)); x != 0 {
			return x
		}
		return strings.Compare(a.key, b.key)
	})
	people := make([]deskPerson, len(out))
	for i, d := range out {
		people[i] = *d
	}
	return people, nil
}

// deskDraft is a message ready to send, or the problem with the form.
type deskDraft struct {
	msg     store.DeskMessage
	problem string
}

// deskFormOf reads the desk form: who it is for, and what to say. A problem
// is something the organiser can put right; err is the store failing.
func (p *competitionPages) deskFormOf(r *http.Request, c store.Competition) (d deskDraft, err error) {
	ctx := r.Context()
	r.ParseForm()
	text := strings.TrimSpace(r.Form.Get("text"))
	d.msg = store.DeskMessage{Text: text, Come: r.Form.Get("come") == "1", Who: linkOf(r).Name}
	switch {
	case text == "" && !d.msg.Come:
		d.problem = "Write a message, or ask them to come to the desk."
		return d, nil
	case len([]rune(text)) > maxDeskText:
		d.problem = fmt.Sprintf("A message can be up to %d characters.", maxDeskText)
		return d, nil
	}
	s := c.Published
	switch to := r.Form.Get("to"); to {
	case "club":
		clubs, err := p.st.CompetitionClubs(ctx, c.ID)
		if err != nil {
			return d, err
		}
		i := slices.IndexFunc(clubs, func(cl store.Club) bool { return cl.ID == r.Form.Get("club") })
		if i < 0 {
			d.problem = "Pick a club."
			return d, nil
		}
		d.msg.Audience, d.msg.Clubs = clubs[i].Name, []string{clubs[i].ID}
	case "flight":
		if s == nil {
			d.problem = "Publish the timetable to message a flight."
			return d, nil
		}
		all, err := p.st.Entries(ctx, c.ID)
		if err != nil {
			return d, err
		}
		keys := personKeys(all)
		// Its gymnasts, its officials (its panel's seats), or both; gymnasts
		// if the form says neither (as before officials could be chosen).
		gymnasts, officials := r.Form.Get("flightGymnasts") == "1", r.Form.Get("flightOfficials") == "1"
		if _, asked := r.Form["flightChoice"]; !asked && !gymnasts && !officials {
			gymnasts = true
		}
		add := func(k string) {
			if !slices.Contains(d.msg.People, k) {
				d.msg.People = append(d.msg.People, k)
			}
		}
		for _, f := range s.Flights {
			if competitions.FlightKey(f) != r.Form.Get("flight") {
				continue
			}
			if gymnasts {
				for _, id := range f.Entries {
					for _, k := range keys[id] {
						add(k)
					}
				}
			}
			if officials {
				for _, o := range f.Officials {
					add(o.Person)
				}
			}
			d.msg.Audience = f.Name()
			switch {
			case gymnasts && officials:
				d.msg.Audience += " (gymnasts and officials)"
			case officials:
				d.msg.Audience += " (officials)"
			}
		}
		switch {
		case !gymnasts && !officials:
			d.problem = "Tick gymnasts, officials or both for the flight."
		case d.msg.Audience == "":
			d.problem = "Pick a flight."
		case len(d.msg.People) == 0:
			d.problem = "No one is in that flight."
		}
	case "officials":
		all, err := p.st.Entries(ctx, c.ID)
		if err != nil {
			return d, err
		}
		officials, err := p.rotaOf(r, c, live(all))
		if err != nil {
			return d, err
		}
		for _, o := range officials {
			d.msg.People = append(d.msg.People, o.Key)
		}
		d.msg.Audience = "All officials"
		if len(d.msg.People) == 0 {
			d.problem = "No officials have offered yet."
		}
	case "panel":
		day, area, _ := strings.Cut(r.Form.Get("panel"), "|")
		i, convErr := strconv.Atoi(day)
		if s == nil || convErr != nil || i < 0 || i >= len(s.Setup.Days) {
			d.problem = "Pick a panel."
			return d, nil
		}
		for _, f := range s.Flights {
			if f.Day != i || f.Area != area {
				continue
			}
			for _, o := range f.Officials {
				if o.Person != "" && !slices.Contains(d.msg.People, o.Person) {
					d.msg.People = append(d.msg.People, o.Person)
				}
			}
		}
		d.msg.Audience = fmt.Sprintf("%s's officials, %s", area, s.Setup.Days[i].Name)
		if len(d.msg.People) == 0 {
			d.problem = "No officials have a seat on that panel yet."
		}
	case "people":
		all, err := p.st.Entries(ctx, c.ID)
		if err != nil {
			return d, err
		}
		known, err := p.deskPeople(r, c, live(all))
		if err != nil {
			return d, err
		}
		var first string
		for _, k := range r.Form["person"] {
			i := slices.IndexFunc(known, func(x deskPerson) bool { return x.key == k })
			if i < 0 || slices.Contains(d.msg.People, k) {
				continue
			}
			if first == "" {
				first = known[i].name
			}
			d.msg.People = append(d.msg.People, k)
		}
		switch n := len(d.msg.People); n {
		case 0:
			d.problem = "Pick at least one person."
		case 1:
			d.msg.Audience = first
		default:
			d.msg.Audience = fmt.Sprintf("%d people", n)
		}
	default:
		d.problem = "Say who it is for."
	}
	return d, nil
}

// describeDesk says what sending a message does, for the history; "" if it
// won't be sent.
func (p *competitionPages) describeDesk(r *http.Request, c store.Competition) string {
	d, err := p.deskFormOf(r, c)
	if err != nil || d.problem != "" {
		return ""
	}
	if d.msg.Come {
		return "Called " + d.msg.Audience + " to the desk"
	}
	return "Sent a message to " + d.msg.Audience
}

// desk is the organisers' desk: the form to send a message, and the
// messages sent.
func (p *competitionPages) desk(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	all, err := p.st.Entries(ctx, c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	entries := live(all)
	people, err := p.deskPeople(r, c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	clubs, err := p.st.CompetitionClubs(ctx, c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	sent, err := p.st.DeskMessages(ctx, c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	page := views.DeskPage{Base: adminPath(r.PathValue("token")), Notice: r.URL.Query().Get("notice"), Competition: summary(c.Competition, p.now())}
	for _, cl := range clubs {
		page.Clubs = append(page.Clubs, views.DeskOption{Value: cl.ID, Label: cl.Name})
	}
	if s := c.Published; s != nil {
		page.Published = true
		for i, d := range s.Setup.Days {
			for _, a := range s.Setup.Areas {
				var group views.DeskGroup
				seats := false
				for _, f := range s.Flights {
					if f.Day != i || f.Area != a.Name {
						continue
					}
					group.Options = append(group.Options, views.DeskOption{Value: competitions.FlightKey(f), Label: competitions.Clock(f.Start) + " · " + f.Name()})
					seats = seats || slices.ContainsFunc(f.Officials, func(o competitions.Duty) bool { return o.Person != "" })
				}
				if len(group.Options) == 0 {
					continue
				}
				group.Label = d.Name + " · " + a.Name
				page.Flights = append(page.Flights, group)
				if seats {
					page.Panels = append(page.Panels, views.DeskOption{Value: strconv.Itoa(i) + "|" + a.Name, Label: a.Name + "'s officials, " + d.Name})
				}
			}
		}
	}
	for _, d := range people {
		page.People = append(page.People, views.DeskPersonOption{Key: d.key, Name: d.name, Detail: d.detail})
	}
	for _, m := range sent {
		page.Sent = append(page.Sent, views.DeskSent{
			At: m.At.In(local).Format("Mon 2 Jan, 15:04"), Who: m.Who, To: m.Audience, Text: m.Full(), Come: m.Come,
			Pushed: m.Pushed, Emailed: m.Emailed,
		})
	}
	render(w, r, views.Desk(page))
}

// sendDesk sends a message (to: club, flight, officials, panel or people,
// with club, flight, panel or person; text; come) and tells those who've
// asked to hear.
func (p *competitionPages) sendDesk(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	back := adminPath(r.PathValue("token")) + "/desk"
	d, err := p.deskFormOf(r, c)
	if err != nil {
		failed(w, r, err)
		return
	}
	if d.problem != "" {
		http.Redirect(w, r, back+"?notice="+url.QueryEscape(d.problem+" Nothing was sent."), http.StatusSeeOther)
		return
	}
	m, err := p.st.SendDeskMessage(r.Context(), c.ID, d.msg)
	if err == store.ErrLimit {
		http.Redirect(w, r, back+"?notice="+url.QueryEscape(fmt.Sprintf("A competition can keep %d messages. Nothing was sent.", store.MaxDeskMessages)), http.StatusSeeOther)
		return
	}
	if err != nil {
		failed(w, r, err)
		return
	}
	pushed, emailed := p.notify.tellDesk(r.Context(), c, m)
	if err := p.st.SetDeskTold(r.Context(), c.ID, m.ID, pushed, emailed); err != nil {
		log.Printf("Desk: %s: %v", c.ID, err)
	}
	notice := fmt.Sprintf("Sent to %s. Told %d by push and %d by email; everyone else sees it on their page.", m.Audience, pushed, emailed)
	http.Redirect(w, r, back+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

// deskNotes are the messages that reach someone, newest first, at most
// maxDeskShown: those sent to any of these people, to any of these clubs (all
// their members), or to the comp sec and coaches of any of these staff clubs.
func (p *competitionPages) deskNotes(ctx context.Context, competitionID string, people, clubs, staff []string) []views.DeskNote {
	sent, err := p.st.DeskMessages(ctx, competitionID)
	if err != nil {
		log.Printf("Desk: %s: %v", competitionID, err)
		return nil
	}
	var out []views.DeskNote
	for _, m := range sent {
		if len(out) == maxDeskShown {
			break
		}
		if m.Reaches(people, clubs, staff) {
			out = append(out, views.DeskNote{Text: m.Full(), At: deskWhen(m.At, p.now()), Come: m.Come})
		}
	}
	return out
}

// deskWhen is when a message was sent, and for one sent in the last day how
// long ago, e.g. "Sat 27 Feb, 10:42 · 12 min ago": with no read status, how
// recent a message is matters most on the day.
func deskWhen(at, now time.Time) string {
	s := at.In(local).Format("Mon 2 Jan, 15:04")
	switch ago := now.Sub(at); {
	case ago < 0 || ago >= 24*time.Hour:
		return s
	case ago < time.Minute:
		return s + " · just now"
	case ago < time.Hour:
		return fmt.Sprintf("%s · %d min ago", s, int(ago.Minutes()))
	case ago < 2*time.Hour:
		return s + " · 1 hour ago"
	default:
		return fmt.Sprintf("%s · %d hours ago", s, int(ago.Hours()))
	}
}

// clubIDs is a club's id as a list, empty for none.
func clubIDs(id string) []string {
	if id == "" {
		return nil
	}
	return []string{id}
}

// memberKeys are the person keys of some members.
func memberKeys(members []store.Member) []string {
	out := make([]string, len(members))
	for i, m := range members {
		out[i] = "m:" + m.ID
	}
	return out
}
