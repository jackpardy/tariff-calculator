package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

// Taps without signal (roadmap: ready for the dry run). The venue's wifi
// drops, so the day page keeps a tap the phone can't send (static/js/taps.js)
// and sends it when the signal is back, with when it was tapped. The server
// records it at that time rather than when it arrives. And a flight's start,
// Warm-up over and finish can be put right afterwards, for when the paper
// sheet was used.

// maxTapDelay is how long a phone may keep a tap before sending it; older
// ones are refused.
const maxTapDelay = 12 * time.Hour

// tapClockSlack is how far a phone's clock may step back between a tap and
// its sending (as when it sets itself from the network) and the tap still be
// recorded, as sent the moment it was made.
const tapClockSlack = 2 * time.Minute

// tapAt is when a tap on the day page was made. A tap sent as it's made is
// now. One kept on the phone carries the phone's clock when tapped and when
// sent (tapped and sent, in milliseconds); the difference, taken from now,
// puts it on the server's clock, however wrong the phone's is. why says why
// its time is refused, "" if it isn't: in the future, or more than 12 hours
// ago. A tap before the competition's day isn't refused, so the page can be
// tried out beforehand; lateness leaves out times on another date anyway.
func (p *competitionPages) tapAt(r *http.Request) (at time.Time, why string) {
	now := p.now()
	if r.FormValue("tapped") == "" {
		return now, ""
	}
	tapped, err1 := strconv.ParseInt(r.FormValue("tapped"), 10, 64)
	sent, err2 := strconv.ParseInt(r.FormValue("sent"), 10, 64)
	if err1 != nil || err2 != nil {
		return now, "A tap wasn't recorded: its time couldn't be read."
	}
	delay := time.Duration(sent-tapped) * time.Millisecond
	if delay < 0 && delay >= -tapClockSlack {
		delay = 0
	}
	at = now.Add(-delay)
	switch {
	case delay < 0:
		return at, "A tap wasn't recorded: its time is in the future. The phone's clock may have changed while it waited to send."
	case delay > maxTapDelay:
		return at, "A tap from " + at.In(local).Format("Mon 15:04") + " wasn't recorded: it's more than 12 hours old."
	}
	return at, ""
}

// tappedNote says, for the history, when a tap kept on the phone was made:
// " (tapped at 10:02)"; "" for one sent within a minute.
func (p *competitionPages) tappedNote(at time.Time) string {
	if p.now().Sub(at) < time.Minute {
		return ""
	}
	return " (tapped at " + at.In(local).Format("15:04") + ")"
}

// putRight are the times a correction of a flight's times gives: zero for
// not yet, and for a Warm-up over left out.
type putRight struct {
	started, warmUpOver, finished time.Time
}

// flightTimesOf reads the times in a correction of a flight's start, Warm-up
// over and finish: clock times ("10:02") on the flight's day, "" for not yet
// (or, for the warm-up, not known). why says what's wrong with them, "" if
// nothing.
func (p *competitionPages) flightTimesOf(c store.Competition, f competitions.ScheduledFlight, startedText, warmUpText, finishedText string) (t putRight, why string) {
	first, err := c.Day()
	if err != nil {
		return t, "The competition has no date to put the times on."
	}
	day := first.AddDate(0, 0, f.Day)
	read := func(text, what string) (time.Time, string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return time.Time{}, ""
		}
		t, err := time.Parse("15:04", text)
		if err != nil {
			return time.Time{}, "Give the " + what + " time as a clock time, like 10:05."
		}
		return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, local), ""
	}
	if t.started, why = read(startedText, "started"); why != "" {
		return t, why
	}
	if t.warmUpOver, why = read(warmUpText, "warm-up over"); why != "" {
		return t, why
	}
	t.finished, why = read(finishedText, "finished")
	switch {
	case why != "":
	case t.started.IsZero() && !t.finished.IsZero():
		why = "Give a started time too: a flight can't finish without starting."
	case t.started.IsZero() && !t.warmUpOver.IsZero():
		why = "Give a started time too: a flight's warm-up can't be over without starting."
	case !t.warmUpOver.IsZero() && t.warmUpOver.Before(t.started):
		why = "The warm-up over time is before the started time."
	case !t.finished.IsZero() && t.finished.Before(t.started):
		why = "The finished time is before the started time."
	case !t.finished.IsZero() && t.finished.Before(t.warmUpOver):
		why = "The finished time is before the warm-up over time."
	case !p.now().Before(time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, local)) &&
		(t.finished.After(p.now()) || t.warmUpOver.After(p.now()) || t.started.After(p.now())):
		// Once the flight's day has come; before it, to try the page out,
		// any time will do.
		why = "The times can't be later than now."
	}
	return t, why
}

// correction finds the flight a correction of times is for, by its key, and
// reads its times. found is false for a flight not in the published
// timetable; applies is false if the times are what's recorded already.
func (p *competitionPages) correction(r *http.Request, c store.Competition) (f competitions.ScheduledFlight, t putRight, applies, found bool, why string, err error) {
	if c.Published == nil {
		return f, t, false, false, "", nil
	}
	key := r.FormValue("flight")
	for _, g := range c.Published.Flights {
		if competitions.FlightKey(g) == key {
			f, found = g, true
		}
	}
	if !found {
		return f, t, false, false, "", nil
	}
	t, why = p.flightTimesOf(c, f, r.FormValue("started"), r.FormValue("warmUpOver"), r.FormValue("finished"))
	if why != "" {
		return f, t, false, true, why, nil
	}
	times, err := p.st.FlightTimes(r.Context(), c.ID)
	if err != nil {
		return f, t, false, true, "", err
	}
	was := times[key]
	// Compared to the minute, as the page shows them.
	same := func(a, b time.Time) bool {
		return a.IsZero() == b.IsZero() && a.Truncate(time.Minute).Equal(b.Truncate(time.Minute))
	}
	return f, t, !same(t.started, was.Started) || !same(t.warmUpOver, was.WarmUpOver) || !same(t.finished, was.Finished), true, "", nil
}

// correctTimes puts right when a flight started, its warm-up was over and it
// finished (flight: its key; started, warmUpOver and finished: clock times on
// its day, "" for not yet; day: the page to go back to). Lateness, the venue
// screen, the delay "what if" and the notes on attendees' pages all follow
// the times put right.
func (p *competitionPages) correctTimes(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	anchor := flightID(r.FormValue("flight"))
	f, t, applies, found, why, err := p.correction(r, c)
	switch {
	case err != nil:
		failed(w, r, err)
	case !found:
		message(w, r, http.StatusBadRequest, "Not a flight", "That flight isn't in the published timetable. It may have been changed: go back and try again.")
	case why != "":
		noteRefused(w)
		http.Redirect(w, r, backToDay(r, "The times for "+f.Name()+" weren't changed. "+why, anchor), http.StatusSeeOther)
	case !applies:
		noteRefused(w)
		http.Redirect(w, r, backToDay(r, "", anchor), http.StatusSeeOther)
	default:
		if err := p.st.SetFlightTimes(r.Context(), c.ID, competitions.FlightKey(f), t.started, t.warmUpOver, t.finished, linkOf(r).Name); err != nil {
			failed(w, r, err)
			return
		}
		if t.started.IsZero() {
			// Not started clears how far the marshal had got, as Undo does.
			if err := p.st.ClearUp(r.Context(), c.ID, competitions.FlightKey(f)); err != nil {
				failed(w, r, err)
				return
			}
		}
		http.Redirect(w, r, backToDay(r, "", anchor), http.StatusSeeOther)
	}
}

// describeCorrection says what putting a flight's times right does, for the
// history: "Corrected a flight's times: BUCS L3 · flight 1 of 2 (Panel 1):
// started 10:02, warm-up over 10:14, finished 11:15" (the warm-up only where
// it's given); "" if it changes nothing.
func (p *competitionPages) describeCorrection(r *http.Request, c store.Competition) string {
	f, t, applies, found, why, err := p.correction(r, c)
	if err != nil || !found || why != "" || !applies {
		return ""
	}
	clock := func(t time.Time, what string) string {
		if t.IsZero() {
			return "not " + what
		}
		return what + " " + t.In(local).Format("15:04")
	}
	times := clock(t.started, "started")
	if !t.warmUpOver.IsZero() {
		times += ", " + clock(t.warmUpOver, "warm-up over")
	}
	return fmt.Sprintf("Corrected a flight's times: %s (%s): %s, %s", f.Name(), f.Area, times, clock(t.finished, "finished"))
}
