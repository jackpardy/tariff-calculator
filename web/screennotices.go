package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Announcements on the venue screen (roadmap item 7), put up and taken down
// from the organisers' desk.

// maxNotice is the longest announcement, in characters: a line or two across
// a screen.
const maxNotice = 160

// noticeFor are how long an announcement can stay up, in minutes; 0 is until
// it's taken down.
var noticeFor = []views.DeskOption{
	{Value: "15", Label: "for 15 minutes"}, {Value: "30", Label: "for 30 minutes"},
	{Value: "60", Label: "for an hour"}, {Value: "0", Label: "until I take it down"},
}

func (p *competitionPages) registerScreenNotices(handle func(string, http.HandlerFunc)) {
	handle("POST /competitions/admin/{token}/desk/screen", p.showOnScreen)
	handle("POST /competitions/admin/{token}/desk/screen/{id}/remove", p.takeDownNotice)
}

// noticeForm is an announcement as posted (text, for), or what's wrong with it.
func noticeForm(r *http.Request) (text string, minutes int, problem string) {
	text = strings.Join(strings.Fields(r.FormValue("text")), " ")
	minutes, err := strconv.Atoi(r.FormValue("for"))
	switch {
	case text == "":
		return "", 0, "Write the announcement."
	case utf8.RuneCountInString(text) > maxNotice:
		return "", 0, "An announcement can be up to " + strconv.Itoa(maxNotice) + " characters."
	case err != nil || minutes < 0 || minutes > 24*60:
		return "", 0, "Choose how long it shows."
	}
	return text, minutes, ""
}

// showOnScreen puts an announcement on the venue screen.
func (p *competitionPages) showOnScreen(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	back := adminPath(r.PathValue("token")) + "/desk"
	text, minutes, problem := noticeForm(r)
	if problem != "" {
		http.Redirect(w, r, back+"?notice="+url.QueryEscape(problem+" Nothing was shown."), http.StatusSeeOther)
		return
	}
	var until time.Time
	if minutes > 0 {
		until = p.now().Add(time.Duration(minutes) * time.Minute)
	}
	if err := p.st.ShowOnScreen(r.Context(), c.ID, text, until, linkOf(r).Name); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, back+"?notice="+url.QueryEscape("It's on the venue screen within 30 seconds."), http.StatusSeeOther)
}

// takeDownNotice takes an announcement off the venue screen.
func (p *competitionPages) takeDownNotice(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := p.st.TakeDownNotice(r.Context(), c.ID, r.PathValue("id")); err != nil && err != store.ErrNotFound {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"/desk?notice="+url.QueryEscape("Taken off the venue screen."), http.StatusSeeOther)
}

// describeNotice is an announcement for the history.
func describeNotice(r *http.Request) string {
	text, _, problem := noticeForm(r)
	if problem != "" {
		return ""
	}
	return "Put up on the venue screen: " + text
}

// screenNotices are the announcements showing now, for the desk page.
func screenNotices(notices []store.ScreenNotice) []views.DeskNotice {
	var out []views.DeskNotice
	for _, n := range notices {
		v := views.DeskNotice{ID: n.ID, Text: n.Text, Who: n.Who, At: n.At.In(local).Format("15:04")}
		if !n.Until.IsZero() {
			v.Until = n.Until.In(local).Format("15:04")
		}
		out = append(out, v)
	}
	return out
}
