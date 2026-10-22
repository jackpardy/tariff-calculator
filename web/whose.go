package web

import (
	"context"
	"net/http"
	"strings"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Pages for someone at a competition (their timeline, calendar, score
// sheets, notifications, late changes and My competition) sit under four
// kinds of link: a member's, an individual's entry, a comp sec's and a
// coach's. whose works out from the link who is asking and which
// competition, so each feature has one handler for all of them.

// asker is who a person-and-competition page is for, and the competition.
type asker struct {
	c     store.Competition
	kind  string // store.NotifyMember, NotifyIndividual, NotifyClub or NotifyCoach
	owner string // their record's id: the member's, individual's, club's or coach's
	key   string // the person, as the timetable knows them; "" for a comp sec or coach
	name  string // the person's name, or the club's for a comp sec or coach
	club  string // their club's id; "" for an individual
	back  string // their page for the competition
	day   string // their My competition page; "" for a comp sec or coach
	entry store.Entry
}

// backTo names the page back is, for the way back to it: the person's or
// club's page for the competition, by the competition's name, or a coach's
// entries.
func (a asker) backTo() string {
	if a.kind == store.NotifyCoach {
		return "Entries"
	}
	return a.c.Name
}

// person says whether a person is asking, rather than a club's comp sec or
// coach.
func (a asker) person() bool { return a.key != "" }

// clubWide says whether a page is for the person's whole club: always for a
// comp sec or coach, and under a member's link its club- pages.
func (a asker) clubWide(r *http.Request) bool {
	return !a.person() || strings.Contains(r.URL.Path, "/club-")
}

// whose is who the page's link is for, and the competition in it, or a page
// saying why there isn't one.
func (p *competitionPages) whose(w http.ResponseWriter, r *http.Request) (asker, bool) {
	token := r.PathValue("token")
	switch {
	case strings.HasPrefix(r.URL.Path, "/clubs/member/"):
		m, ok := p.member(w, r)
		if !ok {
			return asker{}, false
		}
		c, ok := p.memberCompetition(w, r, m)
		return asker{c: c, kind: store.NotifyMember, owner: m.ID, key: store.MemberKey(m.ID), name: m.Name, club: m.ClubID,
			back: memberCompetitionPath(token, c.ID), day: dayPath(memberPath(token), c.ID)}, ok
	case strings.HasPrefix(r.URL.Path, "/competitions/entry/"):
		e, c, ok := p.own(w, r)
		own := "/competitions/entry/" + token
		return asker{c: c, kind: store.NotifyIndividual, owner: e.IndividualID, key: e.Key(), name: e.Entry.Gymnast,
			back: own, day: own + "/day", entry: e}, ok
	case strings.HasPrefix(r.URL.Path, "/clubs/admin/"):
		club, ok := p.clubAdmin(w, r)
		if !ok {
			return asker{}, false
		}
		c, ok := p.enteredCompetition(w, r, club.ID)
		return asker{c: c, kind: store.NotifyClub, owner: club.ID, name: club.Name, club: club.ID, back: clubCompetitionPath(token, c.ID)}, ok
	default:
		coach, clubName, ok := p.coach(w, r)
		if !ok {
			return asker{}, false
		}
		c, ok := p.enteredCompetition(w, r, coach.ClubID)
		return asker{c: c, kind: store.NotifyCoach, owner: coach.ID, name: clubName, club: coach.ClubID, back: coachPath(token)}, ok
	}
}

// linksFor is whose page a competition's links are on.
type linksFor struct {
	base   string // their page for the competition, which the links are under
	key    string // the person, for their duties; "" for a comp sec or coach
	member bool   // a club's member, who has their club's timetable too
	// Whose desk messages reach them (deskNotes): people, clubs (all their
	// members) and staff (the comp sec and coaches of these clubs).
	people, clubs, staff []string
	tell                 *teller // who a message to the organiser is from; nil for no form
	back                 string  // the page a message comes back to, as tellBack knows it; "" for base
}

// competitionLinks are the links on a person's or club's page for a
// competition: the timetables and calendars once published, notifications,
// their duties and score sheets, the desk's messages, and telling the
// organiser.
func (p *competitionPages) competitionLinks(ctx context.Context, c store.Competition, f linksFor) views.MyLinks {
	l := views.MyLinks{Notify: p.notifyLink(f.base + "/notify"), Duties: dutiesOf(c, f.key),
		Desk: p.deskNotes(ctx, c.ID, f.people, f.clubs, f.staff), Tell: p.tellForm(ctx, c, f.base, f.tell)}
	if l.Tell != nil {
		l.Tell.Back = f.back
	}
	if len(l.Duties) > 0 {
		l.ScoreSheets = f.base + "/score-sheets"
	}
	if c.Published != nil {
		l.Timeline, l.Calendar = f.base+"/timeline", f.base+"/calendar.ics"
		if f.member {
			l.ClubTimeline, l.ClubCalendar = f.base+"/club-timeline", f.base+"/club-calendar.ics"
		}
	}
	return l
}
