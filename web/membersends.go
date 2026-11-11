package web

import (
	"context"
	"net/http"
	"strings"
	"time"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Who sends a club member's entries and requests (2026-11-11): the comp sec,
// as entries always went, unless the club lets its members send some of
// them themselves (four settings, all off until turned on); and a
// competition's organiser can take members' only as their club sends them,
// whatever the club's settings say. memberSends is the one place that's
// decided.

func (p *competitionPages) registerMemberSends(handle func(string, http.HandlerFunc)) {
	handle("POST /clubs/admin/{token}/member-sends", p.setMemberSends)
	handle("POST "+adminPrefix+"/clubs-send", p.setClubsSend)
}

// memberSends is what a club's members send a competition themselves: what
// the club lets them, unless the competition takes only what clubs send.
func memberSends(club store.Club, c store.Competition) store.MemberSends {
	if c.ClubsSend {
		return store.MemberSends{}
	}
	return club.MemberSends
}

// memberSendsNow says in a line what a club's members send themselves.
func memberSendsNow(ms store.MemberSends) string {
	var on []string
	for _, k := range []struct {
		on   bool
		what string
	}{{ms.Entries, "their entries"}, {ms.Changes, "changes to them"}, {ms.LateChanges, "late changes"}, {ms.LateEntries, "requests to enter late"}} {
		if k.on {
			on = append(on, k.what)
		}
	}
	if len(on) == 0 {
		return "Members save their entries and requests; you send them"
	}
	if len(on) == 4 {
		return "Members send their entries, changes and late requests themselves"
	}
	return "Members send " + strings.Join(on, ", ") + " themselves; you send the rest"
}

// setMemberSends saves what the club's members send themselves: entries,
// changes, lateChanges and lateEntries, each 1 for on.
func (p *competitionPages) setMemberSends(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	ms := store.MemberSends{Entries: r.FormValue("entries") == "1", Changes: r.FormValue("changes") == "1",
		LateChanges: r.FormValue("lateChanges") == "1", LateEntries: r.FormValue("lateEntries") == "1"}
	if err := p.st.SetMemberSends(r.Context(), club.ID, ms); err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, clubSettingsPath(r.PathValue("token")), "Saved. "+memberSendsNow(ms)+".")
}

// setClubsSend makes the competition take club members' entries and
// requests only as their clubs send them (on=1), or lets each club's
// settings say.
func (p *competitionPages) setClubsSend(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	on := r.FormValue("on") == "1"
	if err := p.st.SetClubsSend(r.Context(), c.ID, on); err != nil {
		failed(w, r, err)
		return
	}
	notice := "Clubs' settings say again whether their members send their own entries and requests."
	if on {
		notice = "Only clubs send their members' entries and requests now. What members sent already stays."
	}
	backToSettings(w, r, notice)
}

// clubsSendLine is what a club is told where a competition overrides its
// settings: members' entries and requests wait for the club.
func clubsSendLine(c store.Competition) string {
	return c.Name + " only takes entries sent by the club, so members' entries and requests wait for you to send them, whatever your club's settings."
}

// overriding are the club's current competitions that take only what
// clubs send, by name, for its Settings.
func (p *competitionPages) overriding(ctx context.Context, club store.Club, now time.Time) []string {
	comps, err := p.st.ClubCompetitions(ctx, club.ID)
	if err != nil {
		return nil
	}
	var out []string
	for _, c := range comps {
		if day, err := c.Day(); c.ClubsSend && (err != nil || !day.AddDate(0, 0, 1).Before(now)) {
			out = append(out, c.Name)
		}
	}
	return out
}

// memberSendsView is the club's Settings line for what members send.
func (p *competitionPages) memberSendsView(ctx context.Context, club store.Club) views.MemberSendsView {
	ms := club.MemberSends
	return views.MemberSendsView{Now: memberSendsNow(ms), Entries: ms.Entries, Changes: ms.Changes, LateChanges: ms.LateChanges,
		LateEntries: ms.LateEntries, Overriding: p.overriding(ctx, club, p.now())}
}
