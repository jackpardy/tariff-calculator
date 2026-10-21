package store

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"tariffCalculator/competitions"
)

// notWatched are the tables the pages' cache doesn't read from (watched has
// those it does). A new table goes in one or the other.
var notWatched = []string{
	"checkins", "club_levels", "club_skills", "clubs", "coach_qualifications", "competition_links",
	"competition_log", "concerns", "desk_messages", "flight_times", "judge_qualifications",
	"late_requests", "member_offers", "member_routines", "migrations", "notify_subscriptions",
	"official_checkins", "payments", "person_requirements", "scratch_clears", "screen_notices", "settings",
	"shared_ties", "site_feedback", "site_feedback_captures", "skill_marks", "up_now",
}

// Every table is either one the pages' cache reads from, so changes to it
// are counted, or one it doesn't.
func TestWatchedTables(t *testing.T) {
	now := time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite%' ORDER BY name`)
	must(t, err)
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		must(t, rows.Scan(&name))
		tables = append(tables, name)
		_, in := watched[name]
		if in == slices.Contains(notWatched, name) {
			t.Errorf("table %s: put it in watched (the pages read it) or notWatched (they don't), not both or neither", name)
		}
	}
	must(t, rows.Err())
	for _, name := range notWatched {
		if !slices.Contains(tables, name) {
			t.Errorf("notWatched names %s, which isn't a table", name)
		}
	}
	for name := range watched {
		if s.db.columns[name] == nil {
			t.Errorf("watched names %s, which isn't a table or hasn't the columns named", name)
		}
	}
}

// The count of a competition's changes moves after each change to what its
// pages work out from its entries, and not after anything else: a lookup,
// a change to another competition, or a change to rows the pages don't read.
func TestWrites(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	a, _, err := s.CreateCompetition(ctx, competition())
	must(t, err)
	b, _, err := s.CreateCompetition(ctx, competition())
	must(t, err)
	club, admin, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	mo, link, err := s.Join(ctx, club.ID, "Mo")
	must(t, err)
	coach, coachLink, err := s.CreateCoach(ctx, club.ID, "Cat")
	must(t, err)
	must(t, s.AttachClub(ctx, club.ID, a.ID))

	// moves runs a change, failing the test unless it moves the counts of
	// exactly the competitions named.
	moves := func(what string, change func() error, want ...string) {
		t.Helper()
		before := map[string]uint64{a.ID: s.Writes(a.ID), b.ID: s.Writes(b.ID)}
		if err := change(); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		for id, name := range map[string]string{a.ID: "a", b.ID: "b"} {
			if moved := s.Writes(id) != before[id]; moved != slices.Contains(want, name) {
				t.Errorf("%s: competition %s's count moved: %v, want %v", what, name, moved, !moved)
			}
		}
	}

	moves("sending an entry", func() error {
		if err := s.SaveMemberEntry(ctx, mo.ID, a.ID, entry("Mo")); err != nil {
			return err
		}
		_, err := s.Send(ctx, club.ID, a.ID, []string{mo.ID})
		return err
	}, "a")
	entries, err := s.Entries(ctx, a.ID)
	must(t, err)
	if len(entries) != 1 {
		t.Fatalf("%d entries, want 1", len(entries))
	}
	e := entries[0]
	moves("looking things up", func() error {
		if _, err := s.ClubByAdmin(ctx, admin); err != nil {
			return err
		}
		if _, err := s.MemberByLink(ctx, link); err != nil {
			return err
		}
		if _, err := s.CoachByLink(ctx, coachLink); err != nil {
			return err
		}
		_, err := s.Members(ctx, club.ID)
		return err
	})
	moves("a wrong link", func() error {
		if _, err := s.MemberByLink(ctx, "no-such-link"); !errors.Is(err, ErrNotFound) {
			return err
		}
		return nil
	})
	now = now.Add(usedEvery + time.Hour)
	moves("marking a club used again, after a while", func() error {
		_, err := s.MemberByLink(ctx, link)
		return err
	})
	moves("a check-in", func() error { return s.SetCheckin(ctx, a.ID, e.ID, CheckedIn, "desk") })
	moves("clearing a scratch", func() error { return s.ClearScratch(ctx, a.ID, e.Key(), ClearCompeting, "desk") })
	moves("an official's check-in", func() error {
		return s.SetOfficialCheckin(ctx, a.ID, "o:someone", []string{"f1"}, CheckedIn, "desk")
	})
	moves("who's up", func() error {
		_, _, err := s.MoveUp(ctx, a.ID, "f1", competitions.Routine{}, func(competitions.Routine) (competitions.Routine, bool) {
			return competitions.Routine{Entry: e.ID, Round: 1}, true
		}, "marshal")
		return err
	})
	moves("a flight's times", func() error { return s.MarkFlight(ctx, a.ID, "f1", "start", "marshal") })
	moves("a desk message", func() error {
		_, err := s.SendDeskMessage(ctx, a.ID, DeskMessage{Text: "Come to the desk", Who: "desk"})
		return err
	})
	moves("a notice on the screens", func() error { return s.ShowOnScreen(ctx, a.ID, "Lunch", now.Add(time.Hour), "desk") })
	moves("feedback about the app", func() error {
		_, err := s.AddSiteFeedback(ctx, SiteFeedback{Text: "Lovely", CompetitionID: a.ID}, Capture{Picture: []byte("jpeg"), PictureType: "image/jpeg"})
		return err
	})
	moves("a push subscription", func() error {
		_, _, err := s.Subscribe(ctx, Subscription{CompetitionID: a.ID, Kind: NotifyMember, OwnerID: mo.ID, Channel: ByPush, Address: "https://push.example/1", Keys: "{}"})
		return err
	})
	moves("a club setting", func() error { return s.SetCoachesSeeAll(ctx, club.ID, true) })

	moves("the organiser's limits", func() error { return s.SetLimits(ctx, a.ID, map[string]int{"BUCS L3": 1}) }, "a")
	moves("another competition's limits", func() error { return s.SetLimits(ctx, b.ID, map[string]int{"BUCS L3": 2}) }, "b")
	moves("an official added", func() error {
		_, err := s.AddOfficial(ctx, b.ID, "Jo", "", competitions.Offer{Marshal: true})
		return err
	}, "b")
	moves("checking an entry", func() error { return s.MarkChecked(ctx, a.ID, e.ID, true, "") }, "a")
	moves("an individual entry", func() error {
		_, _, err := enter(s, b.ID, entry("Ann"))
		return err
	}, "b")
	// A person may be in any competition: a change to who they are counts
	// for every one.
	moves("a member's coach", func() error { return s.SetMemberCoach(ctx, club.ID, mo.ID, coach.ID) }, "a", "b")
	moves("a member joining", func() error {
		_, _, err := s.Join(ctx, club.ID, "Bo")
		return err
	}, "a", "b")
	moves("deleting a competition", func() error { return s.DeleteCompetition(ctx, b.ID) }, "b")
}

// A hundred people opening their pages at once, while changes are saved,
// all get them: nobody waits on a lock for the daily mark, and the few
// connections are shared without anyone waiting for themselves.
func TestManyAtOnce(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, err := s.CreateCompetition(ctx, competition())
	must(t, err)
	club, _, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	var links []string
	for i := range 20 {
		_, link, err := s.Join(ctx, club.ID, fmt.Sprintf("M%d", i))
		must(t, err)
		links = append(links, link)
	}
	_, coachLink, err := s.CreateCoach(ctx, club.ID, "Cat")
	must(t, err)
	now = now.Add(2 * usedEvery) // so the first lookups mark the club used
	var wg sync.WaitGroup
	errs := make(chan error, 100)
	for i := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			switch i % 5 {
			case 0:
				_, err = s.CoachByLink(ctx, coachLink)
			case 1:
				err = s.SetLimits(ctx, c.ID, map[string]int{"BUCS L3": i})
			default:
				_, err = s.MemberByLink(ctx, links[i%len(links)])
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if st := s.db.Stats(); st.MaxOpenConnections != maxConns || st.OpenConnections > maxConns {
		t.Errorf("connections: %+v, want at most %d", st, maxConns)
	}
}
