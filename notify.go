package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

// Notifications (ADR 0008): before a change that can notify, what's live is
// kept as the competition's baseline; when the grace period is up, the
// worker compares it with what's live then and tells each subscriber whose
// people changed, once per address, summarising every change.

// notifyGrace is how long changes wait before they're told, so a mistake
// published and put right tells no one (ADR 0008 Decision 5).
const notifyGrace = 10 * time.Minute

// notifier tells subscribers about changes.
type notifier struct {
	st   *store.Store
	mail mailer // nil: email isn't offered
	now  func() time.Time
	base string       // the site's address, for links in what's sent
	keys *vapid       // the push keys, once read
	http *http.Client // for pushes; nil for the usual one
}

func newNotifier(st *store.Store) *notifier {
	base := os.Getenv("PUBLIC_URL")
	if base == "" {
		base = "https://tariff.pardy.ie"
	}
	return &notifier{st: st, mail: mailerFromEnv(), now: time.Now, base: strings.TrimSuffix(base, "/")}
}

// notifyState is what's live, as far as anyone can be told about it.
type notifyState struct {
	Entries map[string]entryState `json:"entries"` // by entry id
	Duties  map[string][]string   `json:"duties"`  // each person's duties, by person key
	Names   map[string]string     `json:"names"`   // people's names, by person key
}

// entryState is what can be told about an entry.
type entryState struct {
	Gymnasts string   `json:"gymnasts"`
	Event    string   `json:"event"`
	Keys     []string `json:"keys"`              // its people
	Club     string   `json:"club,omitempty"`    // its club's id
	Flight   string   `json:"flight,omitempty"`  // where it is on the timetable, once published
	Area     string   `json:"area,omitempty"`    //
	Time     string   `json:"time,omitempty"`    // when its flight's warm-up starts, e.g. "Saturday 09:40"
	Checked  bool     `json:"checked,omitempty"` //
	Note     string   `json:"note,omitempty"`    // the organiser's note
	Removal  string   `json:"removal,omitempty"` // store.Removed or store.Held
	Waiting  int      `json:"waiting,omitempty"` // its place on a waiting list
}

// state is what's live for a competition now.
func (n *notifier) state(ctx context.Context, c store.Competition) (notifyState, error) {
	entries, err := n.st.Entries(ctx, c.ID)
	if err != nil {
		return notifyState{}, err
	}
	removed, err := n.st.RemovedEntries(ctx, c.ID)
	if err != nil {
		return notifyState{}, err
	}
	all := append(slices.Clone(entries), removed...)
	keys, names := peopleOf(all)
	out := notifyState{Entries: map[string]entryState{}, Duties: map[string][]string{}, Names: names}
	for _, e := range all {
		es := entryState{
			Gymnasts: e.Entry.Gymnasts(), Event: e.Entry.Level, Keys: keys[e.ID], Club: e.ClubID,
			Checked: !e.CheckedAt.IsZero(), Note: e.Note, Removal: e.Removal, Waiting: e.Waiting,
		}
		if t := c.Published; t != nil {
			if i, ok := t.Find(e.ID); ok {
				f := t.Flights[i]
				es.Flight, es.Area, es.Time = f.Name(), f.Area, dayClock(*t, f.Day, f.Start)
			}
		}
		out.Entries[e.ID] = es
	}
	if t := c.Published; t != nil {
		seen := map[string]bool{}
		for _, f := range t.Staffed() {
			for _, d := range f.Officials {
				if !seen[d.Person] {
					seen[d.Person] = true
					out.Duties[d.Person] = dutiesIn(*t, d.Person)
				}
			}
		}
	}
	return out, nil
}

// dayClock is a time on one of the timetable's days, e.g. "Saturday 09:40".
func dayClock(s competitions.Schedule, day, minute int) string {
	if day < len(s.Setup.Days) {
		return s.Setup.Days[day].Name + " " + competitions.Clock(minute)
	}
	return competitions.Clock(minute)
}

// changing is called before a change that can notify: unless changes are
// already waiting, it keeps what's live as the baseline, due after the grace
// period (or at once on the competition's days, if the organiser chose).
func (n *notifier) changing(ctx context.Context, c store.Competition) {
	if n == nil || !c.NotifyDue.IsZero() {
		return
	}
	subs, err := n.st.CompetitionSubscriptions(ctx, c.ID)
	if err != nil || len(subs) == 0 {
		return // nobody to tell
	}
	st, err := n.state(ctx, c)
	if err != nil {
		log.Printf("Notifications: %s: %v", c.ID, err)
		return
	}
	baseline, err := json.Marshal(st)
	if err != nil {
		return
	}
	due := n.now().Add(notifyGrace)
	if c.NoWait && competitionDay(c, n.now()) {
		due = n.now()
	}
	if err := n.st.MarkChanged(ctx, c.ID, baseline, due); err != nil {
		log.Printf("Notifications: %s: %v", c.ID, err)
	}
}

// competitionDay says whether now is one of the competition's days, in
// Irish and UK time: its date, and the days after it its timetable has.
func competitionDay(c store.Competition, now time.Time) bool {
	first, err := c.Day()
	if err != nil {
		return false
	}
	days := 1
	if c.Timetable != nil && len(c.Timetable.Setup.Days) > days {
		days = len(c.Timetable.Setup.Days)
	}
	today := now.In(local).Format(competitions.DateLayout)
	for i := range days {
		if first.AddDate(0, 0, i).Format(competitions.DateLayout) == today {
			return true
		}
	}
	return false
}

// change is one thing to tell, about one person or entry.
type change struct {
	Topic string   // store.AboutTimetable, AboutDuties or AboutCards
	Keys  []string // the people it's about
	Club  string   // the club of the entry it's about, if any
	Who   string   // whose it is, e.g. "Ann Ryan"
	Line  string   // what changed, e.g. "BUCS L5 Women: now Saturday 10:20, Panel 2 (was Saturday 09:40, Panel 1)"
}

// changes are the differences between a baseline and what's live: where
// each entry is on the timetable, each person's duties and each card, but
// not running orders.
func changes(was, now notifyState) []change {
	var out []change
	ids := make([]string, 0, len(now.Entries))
	for id := range now.Entries {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		e, b := now.Entries[id], was.Entries[id]
		add := func(topic, line string) {
			out = append(out, change{Topic: topic, Keys: e.Keys, Club: e.Club, Who: e.Gymnasts, Line: line})
		}
		if e.Removal == "" && (e.Time != b.Time || e.Area != b.Area) {
			switch {
			case e.Time == "":
				add(store.AboutTimetable, b.Event+": no longer on the timetable")
			case b.Time == "":
				add(store.AboutTimetable, fmt.Sprintf("%s: warm-up %s, %s", e.Flight, e.Time, e.Area))
			default:
				add(store.AboutTimetable, fmt.Sprintf("%s: now warm-up %s, %s (was %s, %s)", e.Flight, e.Time, e.Area, b.Time, b.Area))
			}
		}
		if _, existed := was.Entries[id]; !existed {
			continue // a new entry: nothing about its card to tell yet
		}
		switch {
		case b.Waiting > 0 && e.Waiting == 0 && e.Removal == "":
			add(store.AboutCards, e.Event+": in, off the waiting list")
		case b.Waiting == 0 && e.Waiting > 0 && e.Removal == "":
			add(store.AboutCards, e.Event+": on the waiting list ("+waitingWord(e.Waiting)+")")
		case e.Removal != b.Removal && e.Removal == store.Removed:
			add(store.AboutCards, e.Event+": removed by the organiser")
		case e.Removal != b.Removal && e.Removal == store.Held:
			add(store.AboutCards, e.Event+": on hold, the organiser asks for changes")
		case e.Removal != b.Removal:
			add(store.AboutCards, e.Event+": back in the competition")
		case e.Checked && !b.Checked && e.Note != "":
			add(store.AboutCards, e.Event+": card checked, with a note: "+e.Note)
		case e.Checked && !b.Checked:
			add(store.AboutCards, e.Event+": card checked")
		case e.Note != b.Note && e.Note != "":
			add(store.AboutCards, e.Event+": the organiser's note: "+e.Note)
		}
	}
	people := map[string]bool{}
	for k := range now.Duties {
		people[k] = true
	}
	for k := range was.Duties {
		people[k] = true
	}
	keys := make([]string, 0, len(people))
	for k := range people {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		name := now.Names[k]
		if name == "" {
			name = was.Names[k]
		}
		if name == "" {
			continue // not one of the competition's gymnasts: nobody can have asked
		}
		for _, d := range now.Duties[k] {
			if !slices.Contains(was.Duties[k], d) {
				out = append(out, change{Topic: store.AboutDuties, Keys: []string{k}, Who: name, Line: "officiating: " + d})
			}
		}
		for _, d := range was.Duties[k] {
			if !slices.Contains(now.Duties[k], d) {
				out = append(out, change{Topic: store.AboutDuties, Keys: []string{k}, Who: name, Line: "no longer officiating: " + d})
			}
		}
	}
	return out
}

// audience is who a subscription is told about: its people, by key, and
// for a comp sec, its club.
type audience struct {
	keys map[string]bool
	club string
}

func (a audience) about(c change) bool {
	if a.club != "" && c.Club == a.club {
		return true
	}
	for _, k := range c.Keys {
		if a.keys[k] {
			return true
		}
	}
	return false
}

// audienceOf is whose changes a subscription hears about: a member and an
// individual their own, a comp sec the club's members', a coach the
// gymnasts they coach.
func (n *notifier) audienceOf(ctx context.Context, sub store.Subscription, now notifyState, members map[string][]store.Member) audience {
	a := audience{keys: map[string]bool{}}
	switch sub.Kind {
	case store.NotifyMember:
		a.keys["m:"+sub.OwnerID] = true
	case store.NotifyIndividual:
		if e, ok := now.Entries[sub.OwnerID]; ok && len(e.Keys) > 0 {
			a.keys[e.Keys[0]] = true
		}
	case store.NotifyClub:
		a.club = sub.OwnerID
		for _, m := range n.members(ctx, sub.OwnerID, members) {
			a.keys["m:"+m.ID] = true
		}
	case store.NotifyCoach:
		for _, clubMembers := range members {
			for _, m := range clubMembers {
				if m.CoachID == sub.OwnerID {
					a.keys["m:"+m.ID] = true
				}
			}
		}
	}
	return a
}

// members are a club's members, read once per run.
func (n *notifier) members(ctx context.Context, clubID string, cache map[string][]store.Member) []store.Member {
	if ms, ok := cache[clubID]; ok {
		return ms
	}
	ms, err := n.st.Members(ctx, clubID)
	if err != nil {
		log.Printf("Notifications: members of %s: %v", clubID, err)
	}
	cache[clubID] = ms
	return ms
}

// run tells what's due every half minute, until the server stops.
func (n *notifier) run() {
	for {
		n.tell(context.Background())
		time.Sleep(30 * time.Second)
	}
}

// tell sends every competition's changes that are due.
func (n *notifier) tell(ctx context.Context) {
	due, err := n.st.ClaimDue(ctx)
	if err != nil {
		log.Printf("Notifications: %v", err)
		return
	}
	for _, d := range due {
		if err := n.tellCompetition(ctx, d); err != nil {
			log.Printf("Notifications: %s: %v", d.CompetitionID, err)
		}
	}
}

// tellCompetition compares a competition's baseline with what's live and
// tells each address its changes, at once.
func (n *notifier) tellCompetition(ctx context.Context, d store.Due) error {
	c, err := n.st.Competition(ctx, d.CompetitionID)
	if err != nil {
		return err
	}
	var was notifyState
	if err := json.Unmarshal(d.Baseline, &was); err != nil {
		return err
	}
	now, err := n.state(ctx, c)
	if err != nil {
		return err
	}
	all := changes(was, now)
	if len(all) == 0 {
		return nil
	}
	subs, err := n.st.CompetitionSubscriptions(ctx, c.ID)
	if err != nil {
		return err
	}
	// Every club's members, for coaches.
	members := map[string][]store.Member{}
	for _, e := range now.Entries {
		if e.Club != "" {
			n.members(ctx, e.Club, members)
		}
	}
	type told struct {
		subs  []store.Subscription
		lines map[string][]string // by whose
		order []string
	}
	byAddress := map[string]*told{}
	var addresses []string
	for _, sub := range subs {
		a := n.audienceOf(ctx, sub, now, members)
		for _, ch := range all {
			if !sub.About(ch.Topic) || !a.about(ch) {
				continue
			}
			key := sub.Channel + " " + strings.ToLower(sub.Address)
			t := byAddress[key]
			if t == nil {
				t = &told{lines: map[string][]string{}}
				byAddress[key] = t
				addresses = append(addresses, key)
			}
			if !slices.ContainsFunc(t.subs, func(s store.Subscription) bool { return s.ID == sub.ID }) {
				t.subs = append(t.subs, sub)
			}
			if !slices.Contains(t.lines[ch.Who], ch.Line) {
				if len(t.lines[ch.Who]) == 0 {
					t.order = append(t.order, ch.Who)
				}
				t.lines[ch.Who] = append(t.lines[ch.Who], ch.Line)
			}
		}
	}
	for _, key := range addresses {
		t := byAddress[key]
		var groups []changeGroup
		for _, who := range t.order {
			groups = append(groups, changeGroup{who, t.lines[who]})
		}
		switch t.subs[0].Channel {
		case store.ByEmail:
			if n.mail == nil {
				continue
			}
			subject, body, headers := n.changesEmail(c, groups, t.subs)
			if err := n.mail.send(t.subs[0].Address, subject, body, headers); err != nil {
				log.Printf("Notifications: emailing about %s: %v", c.ID, err)
			}
		case store.ByPush:
			err := n.push(ctx, t.subs[0], pushMessage{Title: c.Name, Body: pushSummary(groups), URL: n.base + t.subs[0].Page})
			if errors.Is(err, errGone) {
				for _, s := range t.subs {
					n.st.DropSubscription(ctx, s.ID)
				}
			} else if err != nil {
				log.Printf("Notifications: pushing about %s: %v", c.ID, err)
			}
		}
	}
	return nil
}

// changeGroup is the changes for one person.
type changeGroup struct {
	Who   string
	Lines []string
}

// pushSummary is a push notification's text: the change, if there's one,
// or how many and whose.
func pushSummary(groups []changeGroup) string {
	count := 0
	var who []string
	for _, g := range groups {
		count += len(g.Lines)
		who = append(who, g.Who)
	}
	if count == 1 {
		return groups[0].Who + " · " + groups[0].Lines[0]
	}
	if len(who) > 3 {
		who = append(who[:3], fmt.Sprintf("%d more", len(who)-3))
	}
	names := strings.Join(who, ", ")
	if i := strings.LastIndex(names, ", "); i >= 0 {
		names = names[:i] + " and " + names[i+2:]
	}
	return fmt.Sprintf("%d changes for %s. Tap to see them.", count, names)
}

// changesEmail is the email telling one address its changes.
func (n *notifier) changesEmail(c store.Competition, groups []changeGroup, subs []store.Subscription) (subject, body string, headers map[string]string) {
	count := 0
	for _, g := range groups {
		count += len(g.Lines)
	}
	subject = "Changes at " + c.Name
	if count == 1 {
		subject = "A change at " + c.Name
	}
	var b strings.Builder
	fmt.Fprintf(&b, "The organiser of %s has made changes you asked to hear about.\n\n", c.Name)
	for _, g := range groups {
		fmt.Fprintf(&b, "%s\n", g.Who)
		for _, l := range g.Lines {
			fmt.Fprintf(&b, "- %s\n", l)
		}
		b.WriteString("\n")
	}
	b.WriteString("See it all:\n")
	for _, s := range subs {
		fmt.Fprintf(&b, "%s%s\n", n.base, s.Page)
	}
	b.WriteString("\nTo stop these emails:\n")
	for _, s := range subs {
		fmt.Fprintf(&b, "%s\n", n.offLink(s.Token))
	}
	return subject, b.String(), map[string]string{
		"List-Unsubscribe":      "<" + n.offLink(subs[0].Token) + ">",
		"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
	}
}

func (n *notifier) offLink(token string) string     { return n.base + "/notify/off/" + token }
func (n *notifier) confirmLink(token string) string { return n.base + "/notify/confirm/" + token }

// confirmEmail asks an address to confirm it before anything else is sent.
func (n *notifier) confirmEmail(c store.Competition, sub store.Subscription, about string) error {
	if n.mail == nil {
		return fmt.Errorf("email is off")
	}
	body := fmt.Sprintf(`Someone, we hope you, asked for this email address to be told about changes at %s, for %s.

To start, confirm it:
%s

If it wasn't you, ignore this email: nothing more will be sent.
`, c.Name, about, n.confirmLink(sub.Token))
	return n.mail.send(sub.Address, "Confirm your email for "+c.Name, body, nil)
}
