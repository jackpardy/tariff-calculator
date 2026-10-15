package web

import (
	"net/http"
	"strconv"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The menu at the top of the organiser's pages (roadmap 2026-10-13): the
// sections of a competition, and under them the current section's pages. It
// offers only what the admin link may open (ADR 0009).

// The organiser's pages, by their path under the admin link, as a menu says
// which is current. A page not in the menu (a single entry) names the page it
// belongs to.
const (
	menuEntries   = ""
	menuPeople    = "/people"
	menuLate      = "/late"
	menuCoaches   = "/coaches"
	menuFees      = "/fees"
	menuTimetable = "/timetable"
	menuSetup     = "/timetable/setup"
	menuOfficials = "/officials"
	menuSimulate  = "/timetable/simulate"
	menuDelay     = "/timetable/delay"
	menuLeave     = "/timetable/leave"
	menuDay       = "/day"
	menuDesk      = "/desk"
	menuScreen    = "/screen"
	menuSettings  = "/settings"
	menuHistory   = "/history"
	menuFeedback  = "/feedback"
)

// menuState is what a competition's menu depends on besides the link: which
// pages exist yet, and the counts that badge them.
type menuState struct {
	Published      bool // the timetable is published: the day's pages open
	Planned        bool // the timetable is planned: the what-ifs have something to work on
	ApproveCoaches bool // coaches are approved, so there's a page to do it on
	LateWaiting    int  // late changes to decide
	CoachesWaiting int  // coaches sent or named, not decided yet
	FeedbackNew    int  // feedback about the app not dealt with yet
}

// menuFor is the menu of a competition's pages for the request's link, with
// current the page being shown.
func (p *competitionPages) menuFor(r *http.Request, c store.Competition, current string) []views.MenuSection {
	a := access(r)
	m := menuState{Published: c.Published != nil, Planned: schedule(c).Planned, ApproveCoaches: c.Signoff && c.ApproveCoaches}
	if a.Settings {
		if late, err := p.st.LateRequests(r.Context(), c.ID); err == nil {
			for _, l := range late {
				if l.Status == store.LateWaiting {
					m.LateWaiting++
				}
			}
		}
	}
	if m.ApproveCoaches && a.Coaches {
		sent, err1 := p.st.CompetitionCoaches(r.Context(), c.ID)
		named, err2 := p.st.CompetitionEntryCoaches(r.Context(), c.ID)
		if err1 == nil && err2 == nil {
			m.CoachesWaiting = coachApprovalForm(c.Competition, sent, named).Waiting
		}
	}
	if a.Feedback {
		m.FeedbackNew, _ = p.st.FeedbackWaiting(r.Context(), c.ID)
	}
	return orgMenu(adminPath(r.PathValue("token")), a, m, current)
}

// orgMenu builds the organiser's menu from the admin link's path, what the
// link may open, and the competition's state. current is the page shown
// (menuEntries, menuLate and so on).
func orgMenu(base string, a views.Access, m menuState, current string) []views.MenuSection {
	item := func(label, path, badge string) views.MenuItem {
		return views.MenuItem{Label: label, Href: base + path, Current: path == current, Badge: badge}
	}
	var sections []views.MenuSection
	// add makes a section of the items it was given, current when one is.
	// Nothing when there are none.
	add := func(label string, items ...views.MenuItem) {
		if len(items) == 0 {
			return
		}
		s := views.MenuSection{Label: label, Href: items[0].Href, Items: items}
		for _, it := range items {
			if it.Current {
				s.Current = true
			}
			if s.Badge == "" {
				s.Badge = it.Badge
			}
		}
		// A badge on the section's tab only when it isn't the current section,
		// whose own pages show it.
		if s.Current {
			s.Badge = ""
		}
		sections = append(sections, s)
	}

	entries := []views.MenuItem{item("Entries", menuEntries, "")}
	if a.People {
		entries = append(entries, item("People", menuPeople, ""))
	}
	if a.Settings {
		entries = append(entries, item("Late changes", menuLate, badge(m.LateWaiting, "to decide")))
	}
	if m.ApproveCoaches && a.Coaches {
		entries = append(entries, item("Coaches", menuCoaches, badge(m.CoachesWaiting, "waiting")))
	}
	if a.Settings {
		entries = append(entries, item("Fees", menuFees, ""))
	}
	add("Entries", entries...)

	// A link that only prints the timetable's sheets (a chair of judges) finds
	// them as buttons on the entries page: printouts have no menu of their own.
	if a.Timetable {
		items := []views.MenuItem{item("Timetable", menuTimetable, ""), item("Setup", menuSetup, "")}
		if a.Officials {
			items = append(items, item("Officials", menuOfficials, ""))
		}
		items = append(items, item("Simulate", menuSimulate, ""))
		if m.Planned {
			items = append(items, item("What if there's a delay", menuDelay, ""), item("What if an official leaves", menuLeave, ""))
		}
		add("Timetable", items...)
	}

	// The day's pages, once the timetable is published, or while on one.
	if m.Published || current == menuDay || current == menuDesk || current == menuScreen {
		var day []views.MenuItem
		if a.Day {
			day = append(day, item("On the day", menuDay, ""))
		}
		if a.Desk {
			day = append(day, item("Desk messages", menuDesk, ""))
		}
		if a.Screen {
			day = append(day, item("Venue screen", menuScreen, ""))
		}
		add("On the day", day...)
	}

	// Feedback about the app, beside the history, for links that may read it.
	var more []views.MenuItem
	if a.Settings {
		more = append(more, item("Settings", menuSettings, ""))
	}
	more = append(more, item("History", menuHistory, ""))
	if a.Feedback {
		more = append(more, item("Feedback", menuFeedback, badge(m.FeedbackNew, "new")))
	}
	if a.Settings {
		add("Settings", more...)
	} else {
		add("History", more...)
	}
	return sections
}

// badge says how many are waiting, e.g. "3 to decide"; "" for none.
func badge(n int, what string) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n) + " " + what
}
