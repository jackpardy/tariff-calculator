package store

import (
	"errors"
	"slices"
	"testing"
	"time"
)

// A message is about several entries and events; the organiser says it's
// received, replies, and resolves it with a note of their own; the sender
// adds to it, a thread, which opens it again; the sender sees their own
// messages, and no one else's.
func TestMessagesAndReplies(t *testing.T) {
	now := time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, err := s.CreateCompetition(ctx, competition())
	must(t, err)
	ann := Sender{Role: RoleMember, Name: "Ann", Club: "UCD", ID: "member:ann"}
	club := Sender{Role: RoleClub, Name: "UCD", Club: "UCD", ID: "club:ucd"}
	m, err := s.TellOrganiser(ctx, c.ID, About{Entries: []string{"e1", "e2"}}, ann, "Both my routines")
	must(t, err)
	now = now.Add(time.Minute)
	k, err := s.TellOrganiser(ctx, c.ID, About{Events: []string{"BUCS L3"}}, club, "When does L3 start?")
	must(t, err)
	flag, err := s.FlagConcern(ctx, c.ID, "", "Chairs", "A loose spring")
	must(t, err)

	sent, err := s.SentBy(ctx, c.ID, ann.ID)
	must(t, err)
	if len(sent) != 1 || sent[0].ID != m.ID || !slices.Equal(sent[0].Entries, []string{"e1", "e2"}) || !sent[0].Message() || sent[0].Acknowledged() {
		t.Errorf("Ann's own: %+v", sent)
	}
	if sent, _ := s.SentBy(ctx, c.ID, club.ID); len(sent) != 1 || !slices.Equal(sent[0].Events, []string{"BUCS L3"}) {
		t.Errorf("the club's own, about an event: %+v", sent)
	}
	if sent, _ := s.SentBy(ctx, c.ID, ""); len(sent) != 0 {
		t.Errorf("no sender, nothing: %+v", sent)
	}

	// Received, once.
	now = now.Add(time.Minute)
	must(t, s.Acknowledge(ctx, c.ID, m.ID, "Organiser"))
	if err := s.Acknowledge(ctx, c.ID, m.ID, "Organiser"); !errors.Is(err, ErrNotFound) {
		t.Errorf("received twice: %v", err)
	}
	if got, _ := s.Concern(ctx, c.ID, m.ID); !got.Acknowledged() || got.AckBy != "Organiser" || !got.AckAt.Equal(now) {
		t.Errorf("received: %+v", got)
	}

	// A reply says it's received too; a concern from an admin link takes none.
	now = now.Add(time.Minute)
	r, err := s.ReplyTo(ctx, c.ID, k.ID, "Mary", "At 9:00, panel 1")
	must(t, err)
	if _, err := s.ReplyTo(ctx, c.ID, flag.ID, "Mary", "Fixed"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a reply to a flag: %v", err)
	}
	if _, err := s.ReplyTo(ctx, c.ID, "nope", "Mary", "Hi"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a reply to nothing: %v", err)
	}
	got, _ := s.Concern(ctx, c.ID, k.ID)
	if len(got.Thread) != 1 || got.Thread[0] != r || r.BySender || got.AckBy != "Mary" || !got.Acknowledged() || got.Waiting() {
		t.Errorf("replied: %+v", got)
	}

	// The sender adds to it: theirs, and only theirs.
	now = now.Add(time.Minute)
	more, err := s.AddToMessage(ctx, c.ID, k.ID, club.ID, "And L4?")
	must(t, err)
	if _, err := s.AddToMessage(ctx, c.ID, k.ID, ann.ID, "Not mine"); !errors.Is(err, ErrNotFound) {
		t.Errorf("adding to someone else's: %v", err)
	}
	if _, err := s.AddToMessage(ctx, c.ID, flag.ID, "", "A flag"); !errors.Is(err, ErrNotFound) {
		t.Errorf("adding to a flag: %v", err)
	}
	got, _ = s.Concern(ctx, c.ID, k.ID)
	if len(got.Thread) != 2 || !got.Thread[1].BySender || got.Thread[1].Who != "UCD" || got.Thread[1] != more || !got.Waiting() {
		t.Errorf("a thread, the last word theirs: %+v", got.Thread)
	}
	for range MaxPosts - 2 {
		_, err := s.ReplyTo(ctx, c.ID, k.ID, "Mary", "More")
		must(t, err)
	}
	if _, err := s.ReplyTo(ctx, c.ID, k.ID, "Mary", "One too many"); !errors.Is(err, ErrLimit) {
		t.Errorf("past the thread's length: %v", err)
	}

	// Resolved with a note of the organiser's own; it can't be received after.
	must(t, s.ResolveConcern(ctx, c.ID, m.ID, "Organiser", "Swapped the cards"))
	if err := s.Acknowledge(ctx, c.ID, k.ID, "Organiser"); !errors.Is(err, ErrNotFound) {
		t.Errorf("received after a reply: %v", err)
	}
	all, err := s.Concerns(ctx, c.ID)
	must(t, err)
	if len(all) != 3 || all[len(all)-1].ID != m.ID || all[len(all)-1].Resolution != "Swapped the cards" || len(all[1].Thread) != MaxPosts {
		t.Errorf("open first, with threads: %+v", all)
	}

	// More from Ann opens hers again; still received.
	_, err = s.AddToMessage(ctx, c.ID, m.ID, ann.ID, "It's still wrong")
	must(t, err)
	if got, _ := s.Concern(ctx, c.ID, m.ID); !got.Open() || !got.Acknowledged() || !got.Waiting() || got.Resolution != "Swapped the cards" {
		t.Errorf("open again: %+v", got)
	}
}
