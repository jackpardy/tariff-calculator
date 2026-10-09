package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

// Add to calendar (roadmap 2026-10-09, "For attendees"): a person's or a
// club's flights and duties from the published timetable as an iCalendar
// file. The links are secret and stay the same, so a calendar app can
// subscribe to one and pick up the changes when the timetable is published
// again; until then it is an empty calendar, which fills in later.

func (p *competitionPages) registerCalendars(handle func(string, http.HandlerFunc)) {
	handle("GET /clubs/member/{token}/competitions/{id}/calendar.ics", p.memberCalendar)
	handle("GET /competitions/entry/{token}/calendar.ics", p.individualCalendar)
	handle("GET /clubs/member/{token}/competitions/{id}/club-calendar.ics", p.memberClubCalendar)
	handle("GET /clubs/admin/{token}/competitions/{id}/calendar.ics", p.adminClubCalendar)
	handle("GET /clubs/coach/{token}/competitions/{id}/calendar.ics", p.coachClubCalendar)
}

func (p *competitionPages) memberCalendar(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	c, ok := p.memberCompetition(w, r, m)
	if !ok {
		return
	}
	p.personalCalendar(w, r, c, "m:"+m.ID, m.Name)
}

func (p *competitionPages) individualCalendar(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.own(w, r)
	if !ok {
		return
	}
	p.personalCalendar(w, r, c, individualKey(e.Entry.Gymnast), e.Entry.Gymnast)
}

func (p *competitionPages) memberClubCalendar(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	c, ok := p.memberCompetition(w, r, m)
	if !ok {
		return
	}
	p.clubCalendar(w, r, c, m.ClubID)
}

func (p *competitionPages) adminClubCalendar(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	c, ok := p.enteredCompetition(w, r, club.ID)
	if !ok {
		return
	}
	p.clubCalendar(w, r, c, club.ID)
}

func (p *competitionPages) coachClubCalendar(w http.ResponseWriter, r *http.Request) {
	coach, _, ok := p.coach(w, r)
	if !ok {
		return
	}
	c, ok := p.enteredCompetition(w, r, coach.ClubID)
	if !ok {
		return
	}
	p.clubCalendar(w, r, c, coach.ClubID)
}

// calendarSlot is a placed flight or block, as the calendar needs it: id is
// stable when the timetable is published again, so an event keeps its UID.
type calendarSlot struct {
	id         string
	title      string
	flight     bool
	day        int
	start, end int
	location   string
	entries    []string // a flight's, by id
	officials  []competitions.Duty
}

// calendarClaim is one event a slot gives someone: they compete in it
// (kind "compete") or hold a seat on its panel ("duty", with the role). who
// tells claims on the same slot apart (empty on a personal calendar), name is
// shown in the summary, and note is added to the description.
type calendarClaim struct {
	kind, who, name, role, note string
}

// calendarEvent is one VEVENT.
type calendarEvent struct {
	uid, summary, description, location string
	start, end                          time.Time
}

// calendarSlots are every placed flight and block of a schedule: a block on
// several areas is one slot.
func calendarSlots(s competitions.Schedule) []calendarSlot {
	var out []calendarSlot
	for _, f := range s.Flights {
		id := fmt.Sprintf("flight|%s|%s|%s|%d", f.Discipline, f.Level, f.Category, f.Number)
		out = append(out, calendarSlot{id: id, title: f.Name(), flight: true, day: f.Day, start: f.Start, end: f.End, location: f.Area, entries: f.Entries, officials: f.Officials})
	}
	for _, b := range s.Blocks {
		out = append(out, calendarSlot{id: fmt.Sprintf("block|%s|%d", b.Name, b.Day), title: b.Name, day: b.Day, start: b.Start, end: b.End, location: strings.Join(b.Areas, ", "), officials: b.Officials})
	}
	return out
}

// calendarEventsOf are the events claims makes of a schedule's slots, in
// time order. whose is who the calendar is for, for the UIDs.
func calendarEventsOf(c store.Competition, s competitions.Schedule, whose string, claims func(calendarSlot) []calendarClaim) []calendarEvent {
	first, err := c.Day()
	if err != nil {
		return nil
	}
	at := func(day, minutes int) time.Time {
		return time.Date(first.Year(), first.Month(), first.Day()+day, 0, minutes, 0, 0, local).UTC()
	}
	clock := func(minutes int) string { return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60) }
	var out []calendarEvent
	seen := map[string]bool{}
	for _, slot := range calendarSlots(s) {
		for _, cl := range claims(slot) {
			sum := sha256.Sum256([]byte(strings.Join([]string{c.ID, whose, cl.kind, slot.id, cl.role, cl.who}, "\x00")))
			uid := hex.EncodeToString(sum[:])[:32] + "@tariff.pardy.ie"
			if seen[uid] {
				continue
			}
			seen[uid] = true
			var parts []string
			for _, part := range []string{cl.name, competitions.RoleName(cl.role), slot.title} {
				if part != "" {
					parts = append(parts, part)
				}
			}
			when := "From " + clock(slot.start) + ", finishes about " + clock(slot.end)
			if slot.flight {
				when = "Warm-up " + clock(slot.start) + ", finishes about " + clock(slot.end)
			}
			lines := []string{c.Name}
			if slot.day >= 0 && slot.day < len(s.Setup.Days) && s.Setup.Days[slot.day].Name != "" {
				lines = append(lines, s.Setup.Days[slot.day].Name)
			}
			lines = append(lines, when)
			if cl.note != "" {
				lines = append(lines, cl.note)
			}
			out = append(out, calendarEvent{uid: uid, summary: strings.Join(parts, " · "), description: strings.Join(lines, "\n"),
				location: slot.location, start: at(slot.day, slot.start), end: at(slot.day, slot.end)})
		}
	}
	slices.SortStableFunc(out, func(a, b calendarEvent) int { return a.start.Compare(b.start) })
	return out
}

// icsText escapes a text value (RFC 5545 §3.3.11): backslash, semicolon and
// comma, and line breaks as \n. Other control characters are dropped.
func icsText(s string) string {
	s = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\' || r == ';' || r == ',':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r < ' ' && r != '\t' || r == 0x7f:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// icsFold folds a content line at 75 octets (RFC 5545 §3.1): a line break
// and one space before the rest, never inside a character. The result has no
// final line break.
func icsFold(line string) string {
	var b strings.Builder
	n := 0 // octets on the current line
	for _, r := range line {
		w := utf8.RuneLen(r)
		if n+w > 75 {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += w
	}
	return b.String()
}

// icsTime is a time as UTC, e.g. 20300316T093000Z.
func icsTime(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// icsFile is the calendar, called name, of events, stamped at stamp.
func icsFile(name string, events []calendarEvent, stamp time.Time) []byte {
	var b strings.Builder
	line := func(s string) { b.WriteString(icsFold(s) + "\r\n") }
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//tariff.pardy.ie//competitions//EN")
	line("CALSCALE:GREGORIAN")
	line("METHOD:PUBLISH")
	line("X-WR-CALNAME:" + icsText(name))
	line("REFRESH-INTERVAL;VALUE=DURATION:PT1H")
	line("X-PUBLISHED-TTL:PT1H")
	for _, e := range events {
		line("BEGIN:VEVENT")
		line("UID:" + e.uid)
		line("DTSTAMP:" + icsTime(stamp))
		line("DTSTART:" + icsTime(e.start))
		line("DTEND:" + icsTime(e.end))
		line("SUMMARY:" + icsText(e.summary))
		if e.location != "" {
			line("LOCATION:" + icsText(e.location))
		}
		line("DESCRIPTION:" + icsText(e.description))
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return []byte(b.String())
}

// calendarFileName makes a name safe for a download: lower case letters and digits,
// the rest dashes.
func calendarFileName(s string) string {
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	if out := strings.Trim(b.String(), "-"); out != "" {
		return out
	}
	return "calendar"
}

// serveCalendar writes the calendar for whose (a key, for the UIDs), called
// "<competition> · label", of the events claims picks from the published
// timetable: empty until it is published.
func (p *competitionPages) serveCalendar(w http.ResponseWriter, r *http.Request, c store.Competition, whose, label string,
	claims func(entries []store.Entry, officials []competitions.RotaPerson, name func(string) string) func(calendarSlot) []calendarClaim) {
	var events []calendarEvent
	if t := c.Published; t != nil {
		all, err := p.st.Entries(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		entries := live(all)
		officials, err := p.rotaOf(r, c, entries)
		if err != nil {
			failed(w, r, err)
			return
		}
		_, names := peopleOf(entries)
		events = calendarEventsOf(c, *t, whose, claims(entries, officials, officialNames(officials, names, false)))
	}
	name := c.Name + " · " + label
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="`+calendarFileName(name)+`.ics"`)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(icsFile(name, events, p.now().UTC()))
}

// personalCalendar is a person's (by key, called name): the flights they
// compete in, as a synchro partner too, and the seats they hold.
func (p *competitionPages) personalCalendar(w http.ResponseWriter, r *http.Request, c store.Competition, key, name string) {
	p.serveCalendar(w, r, c, key, name, func(entries []store.Entry, _ []competitions.RotaPerson, _ func(string) string) func(calendarSlot) []calendarClaim {
		keys := personKeys(entries)
		byID := map[string]store.Entry{}
		for _, e := range entries {
			byID[e.ID] = e
		}
		return func(s calendarSlot) []calendarClaim {
			var out []calendarClaim
			for _, id := range s.entries {
				if slices.Contains(keys[id], key) {
					out = append(out, calendarClaim{kind: "compete", name: byID[id].Entry.Gymnasts()})
				}
			}
			for _, d := range s.officials {
				if d.Person == key {
					out = append(out, calendarClaim{kind: "duty", role: d.Role})
				}
			}
			return out
		}
	})
}

// clubCalendar is a club's: every flight one of its gymnasts competes in, or
// one of its people officiates, named.
func (p *competitionPages) clubCalendar(w http.ResponseWriter, r *http.Request, c store.Competition, clubID string) {
	clubName, err := p.st.ClubName(r.Context(), clubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	members, err := p.st.Members(r.Context(), clubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	ours := map[string]bool{} // the club's people, by key
	for _, m := range members {
		ours["m:"+m.ID] = true
	}
	p.serveCalendar(w, r, c, "club:"+clubID, clubName, func(entries []store.Entry, officials []competitions.RotaPerson, name func(string) string) func(calendarSlot) []calendarClaim {
		for _, o := range officials {
			if o.Club == clubName {
				ours[o.Key] = true // the organiser's judges from the club too
			}
		}
		byID := map[string]store.Entry{}
		for _, e := range entries {
			byID[e.ID] = e
		}
		return func(s calendarSlot) []calendarClaim {
			var out []calendarClaim
			for _, id := range s.entries {
				if e, ok := byID[id]; ok && e.ClubID == clubID {
					out = append(out, calendarClaim{kind: "compete", who: e.ID, name: e.Entry.Gymnasts(), note: "Competing: " + e.Entry.Gymnasts()})
				}
			}
			for _, d := range s.officials {
				if ours[d.Person] {
					out = append(out, calendarClaim{kind: "duty", who: d.Person, name: name(d.Person), role: d.Role,
						note: "Officiating: " + name(d.Person) + " (" + competitions.RoleName(d.Role) + ")"})
				}
			}
			return out
		}
	})
}
