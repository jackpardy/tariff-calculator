package store

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"tariffCalculator/competitions"
)

// l4 is entry, moved to BUCS L4.
func l4(gymnast string) competitions.Entry {
	e := entry(gymnast)
	e.Level = "BUCS L4"
	e.Exercises[0].Option, e.Exercises[1].Option = "builtin:bucs-l4-option-1", "builtin:bucs-l4-second"
	return e
}

// lateCompetition is a competition allowing level changes, with one
// individual's entry made while entries were open, on a store whose clock
// reads *now.
func lateCompetition(t *testing.T, now *time.Time) (*Store, Competition, Entry) {
	t.Helper()
	s := open(t, now)
	was := *now
	*now = time.Date(2027, 3, 1, 8, 0, 0, 0, time.UTC)
	defer func() { *now = was }()
	c := competition()
	c.Levels = append(c.Levels, competitions.Level{Ref: "builtin-level:bucs-l4"})
	got, _, err := s.CreateCompetition(ctx, c)
	must(t, err)
	e, _, err := enter(s, got.ID, entry("Ann"))
	must(t, err)
	must(t, s.SetLate(ctx, got.ID, competitions.LateChanges{Rules: map[string]competitions.LateRule{competitions.LateLevel: {On: true, Fee: 500}}}))
	return s, got, e
}

// Late changes on the store's clock: asked, signed off, accepted (the entry
// changes and the fee is kept) or rejected, each decided once.
func TestLateChangesStore(t *testing.T) {
	now := time.Date(2027, 3, 8, 10, 0, 0, 0, time.UTC)
	s, c, e := lateCompetition(t, &now)

	ann := Sender{Role: RoleIndividual, Name: "Ann", ID: "individual:" + e.Key()}
	if _, err := s.RequestLateChange(ctx, c.ID, e.ID, entry("Ann Changed"), "Ann", "", ann); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("routine changes aren't allowed: %v", err)
	}
	r, err := s.RequestLateChange(ctx, c.ID, e.ID, l4("Ann"), "Ann", "Moved up", ann)
	must(t, err)
	if r.Kind != competitions.LateLevel || r.Status != LateWaiting || !r.At.Equal(now) || r.Thread == "" {
		t.Errorf("a level change for the organiser at once, asked now, with a thread: %+v", r)
	}
	if k, err := s.Concern(ctx, c.ID, r.Thread); err != nil || k.Sender != ann.ID || k.Who != "Ann" {
		t.Errorf("the thread is Ann's: %+v, %v", k, err)
	}
	if all, _ := s.Concerns(ctx, c.ID); len(all) != 0 {
		t.Errorf("a request's thread isn't among the messages and concerns: %+v", all)
	}
	if mine, _ := s.SentBy(ctx, c.ID, ann.ID); len(mine) != 0 {
		t.Errorf("nor among the messages Ann sent: %+v", mine)
	}
	if _, err := s.ReplyTo(ctx, c.ID, r.Thread, "Organiser", "Can be accepted if signed off by Friday"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddToMessage(ctx, c.ID, r.Thread, ann.ID, "", "Will do"); err != nil {
		t.Fatal(err)
	}
	if k, _ := s.Concern(ctx, c.ID, r.Thread); len(k.Thread) != 2 || k.Thread[0].BySender || !k.Thread[1].BySender {
		t.Errorf("a reply and an answer: %+v", k.Thread)
	}

	now = now.Add(time.Hour)
	must(t, s.SignLateRequest(ctx, c.ID, r.ID, "Coach C", "", "Seen it"))
	got, ok, err := s.LatestLateRequest(ctx, c.ID, e.ID)
	if err != nil || !ok || got.Status != LateWaiting || got.SignedBy != "Coach C" || !got.SignedAt.Equal(now) || got.SignNote != "Seen it" {
		t.Errorf("signed off now, for the organiser: %+v", got)
	}
	if err := s.SignLateRequest(ctx, c.ID, "nonsense", "Coach C", "", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("no such request: %v", err)
	}

	now = now.Add(time.Hour)
	accepted, err := s.AcceptLateChange(ctx, c.ID, r.ID, "Fine", "Organiser", 500)
	must(t, err)
	if accepted.Status != LateAccepted || accepted.Fee != 500 || accepted.DecidedBy != "Organiser" || !accepted.DecidedAt.Equal(now) || accepted.Reason != "Fine" {
		t.Errorf("accepted now, with the fee: %+v", accepted)
	}
	if k, _ := s.Concern(ctx, c.ID, r.Thread); k.Open() || k.Resolution != "Fine" {
		t.Errorf("deciding closes the thread, with the note: %+v", k)
	}
	changed, err := s.CompetitionEntry(ctx, c.ID, e.ID)
	must(t, err)
	if changed.Entry.Level != "BUCS L4" || changed.SignedBy != "Coach C" {
		t.Errorf("the entry is the one asked for, signed off by the coach: %+v", changed)
	}
	if _, err := s.AcceptLateChange(ctx, c.ID, r.ID, "", "Organiser", 500); !errors.Is(err, ErrNotFound) {
		t.Errorf("accepted once: %v", err)
	}
	if err := s.RejectLateChange(ctx, c.ID, r.ID, "No", "Organiser"); !errors.Is(err, ErrNotFound) {
		t.Errorf("an accepted change can't be rejected: %v", err)
	}

	// Asked again and rejected.
	now = now.Add(time.Hour)
	again, err := s.RequestLateChange(ctx, c.ID, e.ID, entry("Ann"), "Ann", "", ann)
	must(t, err)
	if again.Thread == r.Thread {
		t.Error("a request after one decided has a thread of its own")
	}
	replaced, err := s.RequestLateChange(ctx, c.ID, e.ID, entry("Ann"), "Ann", "Back down", ann)
	must(t, err)
	if replaced.Thread != again.Thread {
		t.Error("a request replacing one still to be decided keeps its thread")
	}
	again = replaced
	now = now.Add(time.Hour)
	must(t, s.RejectLateChange(ctx, c.ID, again.ID, "Full", "Organiser"))
	got, _, _ = s.LatestLateRequest(ctx, c.ID, e.ID)
	if got.ID != again.ID || got.Status != LateRejected || got.Reason != "Full" || !got.DecidedAt.Equal(now) {
		t.Errorf("rejected now, with the reason: %+v", got)
	}
	if all, _ := s.LateRequests(ctx, c.ID); len(all) != 2 {
		t.Errorf("both kept: %+v", all)
	}
	if latest, err := s.LatestLateRequests(ctx, c.ID); err != nil || len(latest) != 1 || !reflect.DeepEqual(latest[e.ID], got) {
		t.Errorf("each entry's latest, at once: %+v, %v; want %+v", latest, err, got)
	}
	if changed, _ := s.CompetitionEntry(ctx, c.ID, e.ID); changed.Entry.Level != "BUCS L4" {
		t.Error("a rejected change leaves the entry as it was")
	}
}

// Late entries on the store's clock: allowed in a competition that never
// said (default on); a member's asked for from their saved entry, out of the
// competition until accepted, then in it, behind those already in (so on
// the waiting list of a full level); an individual's turned down, staying
// out; neither asked again once decided.
func TestLateEntriesStore(t *testing.T) {
	now := time.Date(2027, 3, 8, 10, 0, 0, 0, time.UTC)
	s, c, _ := lateCompetition(t, &now)
	if got, _ := s.Competition(ctx, c.ID); !got.Late.Allowed(competitions.LateNew) || got.Late.Allowed(competitions.LateRoutines) {
		t.Errorf("late entries are allowed unless turned off, the rest only when on: %+v", got.Late)
	}
	club, _, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	mo, _, err := s.Join(ctx, club.ID, "Mo")
	must(t, err)
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	was := now
	now = time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)
	must(t, s.SaveMemberEntry(ctx, mo.ID, c.ID, entry("Mo")))
	now = was
	must(t, s.SetLimits(ctx, c.ID, map[string]int{entry("Ann").Event(): 1}))

	from := Sender{Role: RoleMember, Name: "Mo", Club: "UCD", ID: "member:" + mo.ID}
	r, err := s.AskToEnterLate(ctx, c.ID, mo.ID, "", nil, "Mo", "Sorry, forgot", from)
	must(t, err)
	if r.Kind != competitions.LateNew || !r.Open() || r.Thread == "" {
		t.Errorf("a late entry, to decide, with a thread: %+v", r)
	}
	if in, _ := s.Entries(ctx, c.ID); len(in) != 1 {
		t.Errorf("Mo isn't in the competition yet: %d entries", len(in))
	}
	asked, _ := s.AskedEntries(ctx, c.ID)
	if len(asked) != 1 || asked[0].ID != r.EntryID || asked[0].Removal != Asked {
		t.Fatalf("Mo's copy, asked for: %+v", asked)
	}
	again, err := s.AskToEnterLate(ctx, c.ID, mo.ID, "", nil, "Mo", "", from)
	must(t, err)
	if again.EntryID != r.EntryID || again.Thread != r.Thread {
		t.Errorf("asked again, the same copy and thread: %+v", again)
	}
	now = now.Add(time.Hour)
	if _, err := s.AcceptLateChange(ctx, c.ID, again.ID, "", "Organiser", 700); err != nil {
		t.Fatal(err)
	}
	in, _ := s.Entries(ctx, c.ID)
	i := slices.IndexFunc(in, func(e Entry) bool { return e.ID == r.EntryID })
	if len(in) != 2 || i < 0 || in[i].Waiting != 1 {
		t.Errorf("Mo is in, first on the full level's waiting list: %+v", in)
	}
	if _, err := s.AskToEnterLate(ctx, c.ID, mo.ID, "", nil, "Mo", "", from); !errors.Is(err, ErrEntered) {
		t.Errorf("in already: %v", err)
	}

	bo, _, req, err := s.AskToEnterLateOnOwn(ctx, c.ID, "", entry("Bo"), "")
	must(t, err)
	if bo.Removal != Asked || req.EntryID != bo.ID || req.Kind != competitions.LateNew {
		t.Fatalf("Bo's entry, asked for: %+v %+v", bo, req)
	}
	if k, _ := s.Concern(ctx, c.ID, req.Thread); k.Sender != "individual:"+bo.Key() {
		t.Errorf("the thread is Bo's, as a message from Bo's page would be: %+v", k)
	}
	must(t, s.RejectLateChange(ctx, c.ID, req.ID, "Full", "Organiser"))
	if got, _ := s.CompetitionEntry(ctx, c.ID, bo.ID); got.Removal != Asked {
		t.Errorf("turned down, Bo stays out: %+v", got)
	}

	must(t, s.SetLate(ctx, c.ID, competitions.LateChanges{Rules: map[string]competitions.LateRule{competitions.LateNew: {Off: true}}}))
	if _, _, _, err := s.AskToEnterLateOnOwn(ctx, c.ID, "", entry("Cy"), ""); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("turned off: %v", err)
	}
}

// Payments on the store's clock: recorded, listed in order, removed once.
func TestPaymentsStore(t *testing.T) {
	now := time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)
	s, c, e := lateCompetition(t, &now)
	payer := "entry:" + e.ID
	must(t, s.AddPayment(ctx, c.ID, payer, 1200, "Transfer", "Organiser"))
	now = now.Add(time.Hour)
	must(t, s.AddPayment(ctx, c.ID, payer, 500, "", "Treasurer"))
	got, err := s.Payments(ctx, c.ID)
	must(t, err)
	if len(got) != 2 || got[0].Amount != 1200 || got[0].Note != "Transfer" || got[0].Who != "Organiser" || !got[1].At.Equal(now) || got[1].Who != "Treasurer" {
		t.Fatalf("two payments, oldest first: %+v", got)
	}
	must(t, s.RemovePayment(ctx, c.ID, got[0].ID))
	if err := s.RemovePayment(ctx, c.ID, got[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("removed once: %v", err)
	}
	if left, _ := s.Payments(ctx, c.ID); len(left) != 1 || left[0].Amount != 500 {
		t.Errorf("one left: %+v", left)
	}
	other, _, _ := s.CreateCompetition(ctx, competition())
	if err := s.RemovePayment(ctx, other.ID, got[1].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("not another competition's to remove: %v", err)
	}
}

// Subscribing on the store's clock, and turning it off by its link.
func TestSubscribeAndUnsubscribe(t *testing.T) {
	now := time.Date(2027, 2, 1, 8, 30, 0, 0, time.UTC)
	s, c, e := lateCompetition(t, &now)
	push := Subscription{CompetitionID: c.ID, Kind: NotifyIndividual, OwnerID: e.ID, Channel: ByPush, Address: "https://push.example/1", Keys: "{}", Topics: []string{AboutTimetable}}
	got, token, err := s.Subscribe(ctx, push)
	must(t, err)
	if !got.Confirmed || token == "" || !got.CreatedAt.Equal(now) {
		t.Errorf("push needs no confirming, made now: %+v", got)
	}
	// The same endpoint again replaces it.
	now = now.Add(time.Minute)
	if _, token2, _ := s.Subscribe(ctx, push); token2 == token {
		t.Error("a new token for the replacement")
	}
	if mine, _ := s.Subscriptions(ctx, c.ID, NotifyIndividual, e.ID); len(mine) != 1 || !mine[0].CreatedAt.Equal(now) {
		t.Errorf("one push subscription, the latest: %+v", mine)
	}
	if err := s.Unsubscribe(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("the replaced one's link does nothing: %v", err)
	}
	mine, _ := s.Subscriptions(ctx, c.ID, NotifyIndividual, e.ID)
	must(t, s.Unsubscribe(ctx, mine[0].Token))
	if mine, _ := s.Subscriptions(ctx, c.ID, NotifyIndividual, e.ID); len(mine) != 0 {
		t.Errorf("turned off: %+v", mine)
	}
	if err := s.Unsubscribe(ctx, mine[0].Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("turned off once: %v", err)
	}
}
