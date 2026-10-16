package web

import (
	"cmp"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/requirements"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The organiser's dashboard (ADR 0004 step 3): every entry, judged and
// filtered, each entry's page and check, the printed cards and the CSV,
// and removing and restoring entries.

// videoStatus is an entry's video for the dashboard: "" when none is needed or
// sent, else missing, provided, OK or need more.
func videoStatus(c competitions.Competition, j judged) string {
	if j.err != nil || c.Video.Need == competitions.VideoNone {
		return ""
	}
	needs := c.VideoNeeds(j.card)
	switch {
	case competitions.VideoMissing(j.Entry.Entry, needs):
		return "missing"
	case j.VideoReview == store.VideoOK:
		return "OK"
	case j.VideoReview == store.VideoMore:
		return "need more"
	case needs[0].Needed || needs[1].Needed || j.Entry.Entry.Exercises[0].Video != "" || j.Entry.Entry.Exercises[1].Video != "":
		return "provided"
	}
	return ""
}

// reviewVideo records the organiser's review of an entry's videos (review:
// "ok", "more" or "" to clear) with a note of what more is needed.
func (p *competitionPages) reviewVideo(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if len([]rune(note)) > 300 {
		note = string([]rune(note)[:300])
	}
	// OK says it of a video: with none linked, there's nothing to say it of.
	if r.FormValue("review") == "ok" {
		e, err := p.st.CompetitionEntry(r.Context(), c.ID, r.PathValue("id"))
		if err != nil {
			failed(w, r, err)
			return
		}
		if !slices.ContainsFunc(e.Entry.Exercises[:], func(ex competitions.Exercise) bool { return ex.Video != "" }) {
			seeOther(w, r, adminPath(r.PathValue("token"))+"/entries/"+r.PathValue("id")+"#video", "There's no video yet to say OK to; nothing was changed.")
			return
		}
	}
	if err := p.st.ReviewVideo(r.Context(), c.ID, r.PathValue("id"), r.FormValue("review"), note); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			failed(w, r, err)
		} else {
			badRequest(w, err)
		}
		return
	}
	// Back to the video box, where it says how it was reviewed.
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"/entries/"+r.PathValue("id")+"#video", http.StatusSeeOther)
}

// judged is a stored entry checked, with who sent it ("individual" for an
// individual) and its problems.
type judged struct {
	store.Entry
	club     string
	card     competitions.Card
	err      error // the entry couldn't be checked (its level is gone)
	problems []string
	video    string // videoStatus
}

// judge checks every entry.
func judge(c competitions.Competition, entries []store.Entry) []judged {
	all := enteredAll(c, entries)
	out := make([]judged, len(entries))
	for i, e := range entries {
		j := judged{Entry: e, club: e.ClubName}
		if e.Individual {
			j.club = "individual"
		}
		j.card, j.err = c.Check(e.Entry)
		j.problems = entryProblems(c, e, all)
		j.video = videoStatus(c, j)
		out[i] = j
	}
	return out
}

// entered is an entry as competitions.EntryProblems works out its problems,
// with its sign-off as the organiser sees it: none once it's withdrawn.
func entered(c competitions.Competition, e store.Entry) competitions.Entered {
	en := competitions.Entered{Entry: e.Entry, People: e.People()}
	if !e.Withdrawn {
		en.Signoff = storedSignoff(c, e).Problem()
	}
	return en
}

// enteredAll are a competition's entries that count for a synchro pair's
// individual levels: those not withdrawn.
func enteredAll(c competitions.Competition, entries []store.Entry) []competitions.Entered {
	var out []competitions.Entered
	for _, e := range entries {
		if !e.Withdrawn {
			out = append(out, entered(c, e))
		}
	}
	return out
}

// entryProblems are an entry's problems, as the organiser, its club and its
// coach all see them (competitions.EntryProblems); all are enteredAll. A
// withdrawn entry's pair isn't checked.
func entryProblems(c competitions.Competition, e store.Entry, all []competitions.Entered) []string {
	if e.Withdrawn {
		all = nil
	}
	return c.EntryProblems(entered(c, e), all)
}

// synchroLevels checks each synchro pair's level against their individual
// trampoline levels (by entry index; competitions.PairLevel).
func synchroLevels(c competitions.Competition, entries []store.Entry) map[int]string {
	all := enteredAll(c, entries)
	out := map[int]string{}
	for i, e := range entries {
		if e.Withdrawn {
			continue
		}
		if problem := c.PairLevel(entered(c, e), all); problem != "" {
			out[i] = problem
		}
	}
	return out
}

// matches says whether an entry passes the dashboard's filter: club (a
// club's name, or "individual"), problems=1, unchecked=1, level, entry (an
// id), q (a name, gymnast or synchro partner), coach (their name, or "none"),
// checked and signedoff (yes or no), video (missing, provided, OK or need
// more) and category (Men or Women).
func (j judged) matches(q url.Values) bool {
	yesNo := func(key string, is bool) bool {
		return q.Get(key) == "" || (q.Get(key) == "yes") == is
	}
	coach := q.Get("coach")
	switch {
	case q.Get("club") != "" && q.Get("club") != j.club,
		q.Get("problems") == "1" && (len(j.problems) == 0 || j.Withdrawn),
		q.Get("unchecked") == "1" && (j.Checked() || j.Withdrawn),
		q.Get("level") != "" && q.Get("level") != j.Entry.Entry.Event(),
		q.Get("entry") != "" && q.Get("entry") != j.ID,
		!competitions.NameMatches(j.Entry.Entry.Gymnasts(), q.Get("q")),
		coach == "none" && j.CoachName() != "",
		coach != "" && coach != "none" && coach != j.CoachName(),
		!yesNo("checked", j.Checked()),
		!yesNo("signedoff", j.SignedOff()),
		q.Get("video") != "" && q.Get("video") != j.video,
		q.Get("category") != "" && q.Get("category") != j.Entry.Entry.Category:
		return false
	}
	return true
}

// filterKeys are the dashboard's filters, as matches reads them.
var filterKeys = []string{"club", "problems", "q", "coach", "checked", "signedoff", "video", "category"}

// dashboardFilter is the request's filters, without anything else.
func dashboardFilter(q url.Values) url.Values {
	out := url.Values{}
	for _, k := range filterKeys {
		if v := strings.TrimSpace(q.Get(k)); v != "" {
			out.Set(k, v)
		}
	}
	return out
}

// sortRows orders a level's rows by gymnast, club, sent (latest first) or
// coach; otherwise as they came (by club, then gymnast).
func sortRows(rows []views.DashboardRow, sentAt map[string]time.Time, by string) {
	compare := map[string]func(a, b views.DashboardRow) int{
		"gymnast": func(a, b views.DashboardRow) int { return competitions.CompareNames(a.Gymnast, b.Gymnast) },
		"club":    func(a, b views.DashboardRow) int { return competitions.CompareNames(a.Club, b.Club) },
		"coach": func(a, b views.DashboardRow) int {
			return cmp.Or(competitions.CompareNames(a.Signoff.By, b.Signoff.By), competitions.CompareNames(a.Coach, b.Coach))
		},
	}[by]
	switch {
	case by == "sent":
		slices.SortStableFunc(rows, func(a, b views.DashboardRow) int { return sentAt[b.ID].Compare(sentAt[a.ID]) })
	case compare != nil:
		slices.SortStableFunc(rows, compare)
	}
}

// dashboard lists every entry by level, filtered by club (club: a club's name,
// or "individual") and to entries with problems (problems=1).
func (p *competitionPages) dashboard(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	all, err := p.entriesOf(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	entries, waiting := splitWaiting(all)
	clubs, err := p.st.CompetitionClubs(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	q := r.URL.Query()
	liveDate, liveTime := p.liveFields(c)
	d := views.Dashboard{
		Base: adminPath(r.PathValue("token")), Competition: summary(c, p.now()), Menu: p.menuFor(r, c, menuEntries),
		Club: q.Get("club"), ProblemsOnly: q.Get("problems") == "1",
		Entries: len(entries), New: q.Get("new"), Notice: q.Get("notice"),
		LiveDate: liveDate, LiveTime: liveTime,
	}
	if !c.NotifyDue.IsZero() {
		d.NotifyAt = c.NotifyDue.In(local).Format("15:04")
	}
	d.Waiting = waitingLists(c, waiting, d.Base)
	d.Access = access(r)
	// With no entries yet, the next step: the links to send.
	if len(all) == 0 && d.Access.Settings {
		d.Links.Club = origin(r) + "/competitions/club/" + c.ClubLink
		if c.Individuals {
			d.Links.Individual = origin(r) + "/competitions/enter/" + c.IndividualLink
		}
	}
	if d.Concerns, d.OpenConcerns, err = p.concernViews(r.Context(), c, d.Base); err != nil {
		failed(w, r, err)
		return
	}
	removed, err := p.st.RemovedEntries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	for _, e := range removed {
		d.Removed = append(d.Removed, views.RemovedRow{ID: e.ID, Gymnast: e.Entry.Gymnasts(), Club: clubOf(e), Level: e.Entry.Event(),
			Held: e.Removal == store.Held, Resent: e.Resent, Note: e.RemovalNote, To: reasonTo(e)})
	}
	if d.New != "" && !store.IsVia(r.PathValue("token")) {
		d.Links.Admin = origin(r) + adminPath(r.PathValue("token"))
	}
	seen := map[string]bool{}
	for _, club := range clubs {
		d.Clubs, seen[club.Name] = append(d.Clubs, club.Name), true
	}
	byLevel := map[string]int{}
	for _, name := range c.EventNames() {
		byLevel[name] = len(d.Levels)
		d.Levels = append(d.Levels, views.DashboardLevel{Name: name})
	}
	filter := dashboardFilter(q)
	d.Filter, d.Search, d.Coach, d.CheckedFilter, d.SignedFilter, d.VideoFilter, d.Category, d.Sort =
		filter, filter.Get("q"), filter.Get("coach"), filter.Get("checked"), filter.Get("signedoff"), filter.Get("video"), filter.Get("category"), q.Get("sort")
	d.CanSplit = c.Split.Any()
	coaches := map[string]bool{}
	sentAt := map[string]time.Time{}
	people := personKeys(live(entries)) // whose page each live entry links to
	judgedIn, err := p.judgedOf(r.Context(), c.ID, false)
	if err != nil {
		failed(w, r, err)
		return
	}
	for _, j := range judgedIn {
		if name := j.CoachName(); name != "" && !coaches[name] {
			coaches[name] = true
			d.Coaches = append(d.Coaches, name)
		}

		if !j.Individual && !seen[j.club] {
			d.Clubs, seen[j.club] = append(d.Clubs, j.club), true // a club deleted since it sent
		}
		// A withdrawn entry isn't live: it's listed, marked, but not counted.
		switch {
		case j.Withdrawn:
			d.Entries--
			d.Withdrawn++
		case len(j.problems) > 0:
			d.WithProblems++
		}
		if !j.Checked() && !j.Withdrawn {
			d.Unchecked++
		}
		if !j.matches(filter) {
			continue
		}
		d.Shown++
		sentAt[j.ID] = j.SentAt
		row := views.DashboardRow{
			ID: j.ID, Gymnast: j.Entry.Entry.Gymnasts(), Club: j.club, Problems: j.problems, Coach: j.CoachName(),
			Sent: j.SentAt.In(local).Format("2 Jan, 15:04"), Checked: j.Checked(), Note: j.Note, Withdrawn: j.Withdrawn,
			Video: videoStatus(c.Competition, j), Signoff: storedSignoff(c.Competition, j.Entry), Category: j.Entry.Entry.Category,
		}
		if k := j.Entry.SignerKey(); k != "" {
			row.Signoff.Link = personPath(d.Base, k)
		}
		if ks := people[j.ID]; len(ks) > 0 {
			row.Person = personPath(d.Base, ks[0])
			if len(ks) > 1 {
				row.Partner = personPath(d.Base, ks[1])
			}
		}
		if j.err == nil && !j.card.Unchecked {
			for i, ex := range []requirements.Checked{j.card.First, j.card.Second} {
				if !j.card.Does(i) {
					continue
				}
				row.Met, row.Rules = row.Met+ex.Met(), row.Rules+len(ex.Results)
				row.Exercises[i] = exerciseSummary(j.Entry.Entry.Exercises[i], ex)
			}
		}
		n, ok := byLevel[j.Entry.Entry.Event()]
		if !ok {
			n, byLevel[j.Entry.Entry.Event()] = len(d.Levels), len(d.Levels)
			d.Levels = append(d.Levels, views.DashboardLevel{Name: j.Entry.Entry.Event()})
		}
		d.Levels[n].Rows = append(d.Levels[n].Rows, row)
	}
	slices.SortFunc(d.Coaches, competitions.CompareNames)
	slices.SortFunc(d.Clubs, competitions.CompareNames)
	totals := map[string]int{}
	for _, e := range entries {
		if !e.Withdrawn {
			totals[e.Entry.Event()]++
		}
	}
	for i := range d.Levels {
		d.Levels[i].Anchor, d.Levels[i].Total = "level-"+strconv.Itoa(i), totals[d.Levels[i].Name]
		if limit := c.Limits[d.Levels[i].Name]; limit > 0 {
			d.Levels[i].Places = fmt.Sprintf("%d of %d places", totals[d.Levels[i].Name], limit)
		}
		sortRows(d.Levels[i].Rows, sentAt, d.Sort)
	}
	render(w, r, views.CompetitionDashboard(d))
}

// cards prints the competition cards of the entries the dashboard's filter
// selects (club, problems, unchecked, level, entry), one exercise per page, by
// level then as listed.
func (p *competitionPages) cards(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	page := views.CardsPage{Title: "Cards · " + c.Name, Back: adminPath(r.PathValue("token"))}
	q := r.URL.Query()
	all, err := p.judgedOf(r.Context(), c.ID, false)
	if err != nil {
		failed(w, r, err)
		return
	}
	for _, level := range levelOrder(c.Competition, all) {
		for _, j := range all {
			if j.Entry.Entry.Event() != level || !j.matches(q) || j.err != nil || j.card.Unchecked || (j.Withdrawn && q.Get("entry") == "") {
				continue
			}
			club := j.ClubName
			if j.Individual {
				club = ""
			}
			for i, ex := range []requirements.Checked{j.card.First, j.card.Second} {
				if !j.card.Does(i) {
					continue
				}
				round := [...]string{"1st exercise", "2nd exercise"}[i]
				if j.card.Only > 0 {
					round = "Routine"
				}
				// Its set's name, less what the card says already (the level,
				// "first exercise"): "1st exercise · option 2".
				set := strings.TrimPrefix(ex.SetName, j.Entry.Entry.Event()+" · ")
				if set != "" && !strings.Contains(strings.ToLower(set), "exercise") && set != j.Entry.Entry.Event() {
					round += " · " + set
				}
				page.Cards = append(page.Cards, views.PrintedCard{
					Validation: ex.Validation, Required: ex.Required, Checks: ex.Checks,
					Details: views.SheetDetails{
						"gymnast": j.Entry.Entry.Gymnasts(), "club": club, "category": strings.TrimSpace(j.Entry.Entry.Event() + " " + j.Entry.Entry.Category),
						"competition": c.Name, "round": round,
						"coach": j.CoachName(),
					},
				})
			}
		}
	}
	render(w, r, views.CompetitionCards(page))
}

// levelOrder is the competition's levels in order, then any level entries
// name that it no longer offers.
func levelOrder(c competitions.Competition, entries []judged) []string {
	var out []string
	seen := map[string]bool{}
	for _, name := range c.EventNames() {
		if !seen[name] {
			out, seen[name] = append(out, name), true
		}
	}
	for _, j := range entries {
		if name := j.Entry.Entry.Event(); !seen[name] {
			out, seen[name] = append(out, name), true
		}
	}
	return out
}

// csvExport is every entry as CSV, for a scoring system or a spreadsheet: one
// row per gymnast, with each exercise's requirements and difficulty.
func (p *competitionPages) csvExport(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	all, err := p.judgedOf(r.Context(), c.ID, false)
	if err != nil {
		failed(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName(c.Name)+".csv"))
	out := csv.NewWriter(w)
	out.Write([]string{"Gymnast", "Club", "Level", "1st exercise", "1st difficulty", "2nd exercise", "2nd difficulty", "Problems", "Checked", "Note", "Sent", "Video", "Withdrawn", "Signed off by", "Category"})
	q := r.URL.Query()
	for _, level := range levelOrder(c.Competition, all) {
		for _, j := range all {
			if j.Entry.Entry.Event() != level || !j.matches(q) {
				continue
			}
			club := j.ClubName
			if j.Individual {
				club = onTheirOwn
			}
			row := []string{j.Entry.Entry.Gymnasts(), club, level, "", "", "", "", strconv.Itoa(len(j.problems)), "", j.Note, j.SentAt.In(local).Format("2006-01-02 15:04"), videoStatus(c.Competition, j), "", "", j.Entry.Entry.Category}
			if j.err == nil && !j.card.Unchecked {
				for i, ex := range []requirements.Checked{j.card.First, j.card.Second} {
					if !j.card.Does(i) {
						continue
					}
					row[3+2*i] = ex.SetName
					if ex.Checks.ScoreDifficulty {
						row[4+2*i] = fmt.Sprintf("%.1f", ex.Validation.TotalTariff)
					}
				}
			}
			if j.Checked() {
				row[8] = "yes"
			}
			if j.Withdrawn {
				row[12] = "yes"
			}
			if j.SignedOff() {
				row[13] = j.SignedBy
			}
			for i := range row {
				row[i] = csvSafe(row[i])
			}
			out.Write(row)
		}
	}
	out.Flush()
}

// check marks an entry checked (checked=1) or not, with a note for the club
// or gymnast.
func (p *competitionPages) check(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if len([]rune(note)) > 500 {
		note = string([]rune(note)[:500])
	}
	p.notify.changing(r.Context(), c)
	if err := p.st.MarkChecked(r.Context(), c.ID, r.PathValue("id"), r.FormValue("checked") == "1", note); err != nil {
		failed(w, r, err)
		return
	}
	base := adminPath(r.PathValue("token"))
	// Mark checked and next: the next entry still to check, in the same
	// level first, as the entries list has them.
	if r.FormValue("next") == "1" {
		entries, err := p.entriesOf(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		if next := nextToCheck(live(entries), r.PathValue("id")); next != "" {
			seeOther(w, r, base+"/entries/"+next+"#check", "Checked. This is the next to check.")
			return
		}
		seeOther(w, r, base, "Checked: that was the last to check.")
		return
	}
	// Back to the check box, where it says Checked.
	http.Redirect(w, r, base+"/entries/"+r.PathValue("id")+"#check", http.StatusSeeOther)
}

// nextToCheck is the entry to check after the one with id: the next in its
// level not checked, else the first in any level; "" when none is left.
func nextToCheck(entries []store.Entry, id string) string {
	i := slices.IndexFunc(entries, func(e store.Entry) bool { return e.ID == id })
	if i < 0 {
		return ""
	}
	level := entries[i].Entry.Event()
	open := func(e store.Entry) bool { return e.ID != id && !e.Checked() && !e.Withdrawn }
	for _, e := range slices.Concat(entries[i+1:], entries[:i]) {
		if e.Entry.Event() == level && open(e) {
			return e.ID
		}
	}
	for _, e := range entries {
		if open(e) {
			return e.ID
		}
	}
	return ""
}

// exerciseSummary is an exercise in a dashboard row: a set routine performed as
// written by its name, anything else by its difficulty.
func exerciseSummary(ex competitions.Exercise, c requirements.Checked) string {
	if len(ex.Skills) == 0 && !c.Checks.ScoreDifficulty {
		return c.SetName
	}
	if !c.Checks.ScoreDifficulty {
		return fmt.Sprintf("%d skills", len(c.Validation.Skills))
	}
	return fmt.Sprintf("%.1f", c.Validation.TotalTariff)
}

// entry shows one entry, checked.
func (p *competitionPages) entry(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	e, err := p.st.CompetitionEntry(r.Context(), c.ID, r.PathValue("id"))
	if err != nil {
		failed(w, r, err)
		return
	}
	shown, err := card(c.Competition, e.Entry, clubOf(e))
	if err != nil {
		failed(w, r, err)
		return
	}
	signoff := storedSignoff(c.Competition, e)
	if k := e.SignerKey(); k != "" {
		signoff.Link = personPath(adminPath(r.PathValue("token")), k)
	}
	withSignoff(&shown, signoff)
	render(w, r, views.CompetitionEntry(views.EntryDetail{
		Base: adminPath(r.PathValue("token")), Competition: summary(c, p.now()), Menu: p.menuFor(r, c, menuEntries),
		Card: shown, Sent: e.SentAt.In(local).Format("Monday 2 January, 15:04"),
		ID: e.ID, Checked: checkedText(e), Note: e.Note, VideoReview: e.VideoReview, VideoNote: e.VideoNote,
		Withdrawn: e.Withdrawn, Access: access(r), Notice: r.URL.Query().Get("notice"), LetIn: e.LetIn,
	}))
}

// checkedText says when an entry was marked checked, "" if it isn't.
func checkedText(e store.Entry) string {
	if !e.Checked() {
		return ""
	}
	return e.CheckedAt.In(local).Format("Monday 2 January, 15:04")
}

// reasonTo says who sees why an entry was removed or held.
func reasonTo(e store.Entry) string {
	switch {
	case e.Individual:
		return "the gymnast sees it"
	case e.RemovalTo == store.ToClub:
		return "the club sees it"
	case e.RemovalTo == store.ToMember:
		return "the gymnast sees it"
	}
	return "the club and gymnast see it"
}

// removeEntries removes (action=remove) or holds (action=hold) the entries
// ticked (entry: their ids), with a reason (note) for the club, the gymnast
// or both (to).
func (p *competitionPages) removeEntries(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	removal, done := store.Removed, "Removed %s."
	if r.FormValue("action") == "hold" {
		removal, done = store.Held, "Put %s on hold."
	}
	to := r.FormValue("to")
	if to != store.ToClub && to != store.ToMember {
		to = store.ToBoth
	}
	p.notify.changing(r.Context(), c)
	n, err := p.st.RemoveEntries(r.Context(), c.ID, r.Form["entry"], removal, limitNote(r.FormValue("note")), to)
	if err != nil {
		failed(w, r, err)
		return
	}
	notice := "Tick the entries to remove or hold first; nothing was changed."
	if n > 0 {
		notice = fmt.Sprintf(done, views.Count(n, "entry", "entries")) + " They're listed at the end, to restore."
	}
	seeOther(w, r, adminPath(r.PathValue("token")), notice)
}

// restoreEntry puts a removed or held entry back in the competition.
func (p *competitionPages) restoreEntry(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	p.notify.changing(r.Context(), c)
	if err := p.st.RestoreEntry(r.Context(), c.ID, r.PathValue("id")); err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, adminPath(r.PathValue("token")), "Back in the competition.")
}
