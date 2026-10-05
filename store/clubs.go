package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"tariffCalculator/competitions"
)

// Club is a stored club. Its join link is shared in the club's chat, so it's
// kept to show again; its admin link is shown once.
type Club struct {
	ID        string
	Name      string
	JoinLink  string
	CreatedAt time.Time
	LastUsed  time.Time
}

// CreateClub stores a club and returns it with its admin link. The name must
// already be checked.
func (s *Store) CreateClub(ctx context.Context, name string) (Club, string, error) {
	now := s.stamp()
	c := Club{ID: newID(), Name: name, JoinLink: newToken(), CreatedAt: parseTime(now), LastUsed: parseTime(now)}
	admin := newToken()
	_, err := s.db.ExecContext(ctx, `INSERT INTO clubs (id, admin_hash, join_token, join_hash, name, created_at, last_used)
		VALUES ($1, $2, $3, $4, $5, $6, $6)`, c.ID, hash(admin), c.JoinLink, hash(c.JoinLink), name, now)
	if err != nil {
		return Club{}, "", fmt.Errorf("storing the club: %w", err)
	}
	return c, admin, nil
}

// clubBy finds a club by one of its links' hashes and marks it used.
func (s *Store) clubBy(ctx context.Context, column, token string) (Club, error) {
	var c Club
	var created, used string
	err := s.db.QueryRowContext(ctx, `UPDATE clubs SET last_used = $1 WHERE `+column+` = $2
		RETURNING id, name, join_token, created_at, last_used`, s.stamp(), hash(token)).
		Scan(&c.ID, &c.Name, &c.JoinLink, &created, &used)
	if err != nil {
		return Club{}, notFound(err)
	}
	c.CreatedAt, c.LastUsed = parseTime(created), parseTime(used)
	return c, nil
}

// ClubByAdmin is the club a comp sec's admin link opens.
func (s *Store) ClubByAdmin(ctx context.Context, token string) (Club, error) {
	return s.clubBy(ctx, "admin_hash", token)
}

// ClubByJoinLink is the club a join link joins.
func (s *Store) ClubByJoinLink(ctx context.Context, token string) (Club, error) {
	return s.clubBy(ctx, "join_hash", token)
}

// ReplaceClubAdmin gives a club a new admin link, which ends the old one.
func (s *Store) ReplaceClubAdmin(ctx context.Context, id string) (string, error) {
	admin := newToken()
	return admin, affected(s.db.ExecContext(ctx, `UPDATE clubs SET admin_hash = $1 WHERE id = $2`, hash(admin), id))
}

// DeleteClub deletes a club, its members and their entries. Entries it already
// sent stay with each competition, under the competition's retention.
func (s *Store) DeleteClub(ctx context.Context, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM clubs WHERE id = $1`, id))
}

// Member is one of a club's members.
type Member struct {
	ID        string
	ClubID    string
	Name      string
	CreatedAt time.Time
}

// Join adds a member to a club and returns them with their personal link. The
// name must already be checked.
func (s *Store) Join(ctx context.Context, clubID, name string) (Member, string, error) {
	now := s.stamp()
	m := Member{ID: newID(), ClubID: clubID, Name: name, CreatedAt: parseTime(now)}
	token := newToken()
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM members WHERE club_id = $1`, clubID).Scan(&n); err != nil {
			return err
		}
		if n >= MaxMembers {
			return ErrLimit
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO members (id, club_id, token_hash, name, created_at) VALUES ($1, $2, $3, $4, $5)`,
			m.ID, clubID, hash(token), name, now); err != nil {
			return err
		}
		return touch(ctx, tx, clubID, now)
	})
	if err != nil {
		return Member{}, "", err
	}
	return m, token, nil
}

// touch marks a club used.
func touch(ctx context.Context, tx *sql.Tx, clubID, now string) error {
	return affected(tx.ExecContext(ctx, `UPDATE clubs SET last_used = $1 WHERE id = $2`, now, clubID))
}

// MemberByLink is the member a personal link belongs to. It marks their club used.
func (s *Store) MemberByLink(ctx context.Context, token string) (Member, error) {
	var m Member
	var created string
	err := s.tx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT id, club_id, name, created_at FROM members WHERE token_hash = $1`, hash(token)).
			Scan(&m.ID, &m.ClubID, &m.Name, &created)
		if err != nil {
			return notFound(err)
		}
		return touch(ctx, tx, m.ClubID, s.stamp())
	})
	m.CreatedAt = parseTime(created)
	return m, err
}

// Members are a club's members, by name.
func (s *Store) Members(ctx context.Context, clubID string) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, club_id, name, created_at FROM members WHERE club_id = $1 ORDER BY name, created_at`, clubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		var created string
		if err := rows.Scan(&m.ID, &m.ClubID, &m.Name, &created); err != nil {
			return nil, err
		}
		m.CreatedAt = parseTime(created)
		out = append(out, m)
	}
	return out, rows.Err()
}

// ReplaceMemberLink gives a club's member a new personal link, which ends the
// old one: for the comp sec to send to a member who lost theirs. Only hashes
// are kept, so the old link can't be shown again.
func (s *Store) ReplaceMemberLink(ctx context.Context, clubID, memberID string) (string, error) {
	token := newToken()
	return token, affected(s.db.ExecContext(ctx, `UPDATE members SET token_hash = $1 WHERE id = $2 AND club_id = $3`, hash(token), memberID, clubID))
}

// RemoveMember removes a member from their club, with their entries. Entries
// already sent stay with each competition.
func (s *Store) RemoveMember(ctx context.Context, clubID, memberID string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM members WHERE id = $1 AND club_id = $2`, memberID, clubID))
}

// AttachClub enters a club in a competition, through the competition's club
// link, so its members can enter it and the comp sec can send their entries.
// Attaching again does nothing.
func (s *Store) AttachClub(ctx context.Context, clubID, competitionID string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM competitions WHERE id = $1`, competitionID).Scan(&exists); err != nil {
			return notFound(err)
		}
		now := s.stamp()
		if _, err := tx.ExecContext(ctx, `INSERT INTO club_competitions (club_id, competition_id, attached_at) VALUES ($1, $2, $3)
			ON CONFLICT (club_id, competition_id) DO NOTHING`, clubID, competitionID, now); err != nil {
			return err
		}
		return touch(ctx, tx, clubID, now)
	})
}

// ClubCompetitions are the competitions a club is entered in, soonest first.
func (s *Store) ClubCompetitions(ctx context.Context, clubID string) ([]Competition, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+"c."+strings.ReplaceAll(competitionColumns, ", ", ", c.")+`
		FROM competitions c JOIN club_competitions cc ON cc.competition_id = c.id
		WHERE cc.club_id = $1 ORDER BY c.date, c.name`, clubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Competition
	for rows.Next() {
		c, err := scanCompetition(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CompetitionClubs are the clubs entered in a competition, by name.
func (s *Store) CompetitionClubs(ctx context.Context, competitionID string) ([]Club, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.name, c.join_token, c.created_at, c.last_used
		FROM clubs c JOIN club_competitions cc ON cc.club_id = c.id
		WHERE cc.competition_id = $1 ORDER BY c.name`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Club
	for rows.Next() {
		var c Club
		var created, used string
		if err := rows.Scan(&c.ID, &c.Name, &c.JoinLink, &created, &used); err != nil {
			return nil, err
		}
		c.CreatedAt, c.LastUsed = parseTime(created), parseTime(used)
		out = append(out, c)
	}
	return out, rows.Err()
}

// MemberEntry is a member's entry for a competition, as kept with their club,
// and whether the club has sent it.
type MemberEntry struct {
	MemberID      string
	MemberName    string
	CompetitionID string
	Entry         competitions.Entry
	UpdatedAt     time.Time
	SentAt        time.Time // zero if never sent
}

// Sent says whether the club has sent this entry to the competition.
func (e MemberEntry) Sent() bool { return !e.SentAt.IsZero() }

// ChangedSinceSent says whether the member changed it after it was sent.
func (e MemberEntry) ChangedSinceSent() bool { return e.Sent() && e.UpdatedAt.After(e.SentAt) }

// SaveMemberEntry keeps a member's validated entry for a competition their
// club is entered in, replacing any they had, until the deadline. The entry
// is under the member's name.
func (s *Store) SaveMemberEntry(ctx context.Context, memberID, competitionID string, e competitions.Entry) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var clubID string
		err := tx.QueryRowContext(ctx, `SELECT m.club_id, m.name FROM members m
			JOIN club_competitions cc ON cc.club_id = m.club_id AND cc.competition_id = $2
			WHERE m.id = $1`, memberID, competitionID).Scan(&clubID, &e.Gymnast)
		if err != nil {
			if err == sql.ErrNoRows {
				return ErrNotAttached
			}
			return err
		}
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if err := s.open(ctx, tx, competitionID); err != nil {
			return err
		}
		now := s.stamp()
		if _, err := tx.ExecContext(ctx, `INSERT INTO member_entries (member_id, competition_id, entry, updated_at) VALUES ($1, $2, $3, $4)
			ON CONFLICT (member_id, competition_id) DO UPDATE SET entry = excluded.entry, updated_at = excluded.updated_at`,
			memberID, competitionID, string(data), now); err != nil {
			return err
		}
		return touch(ctx, tx, clubID, now)
	})
}

// WithdrawMemberEntry removes a member's entry for a competition, until the
// deadline. A copy already sent stays until the club sends again.
func (s *Store) WithdrawMemberEntry(ctx context.Context, memberID, competitionID string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if err := s.open(ctx, tx, competitionID); err != nil {
			return err
		}
		return affected(tx.ExecContext(ctx, `DELETE FROM member_entries WHERE member_id = $1 AND competition_id = $2`, memberID, competitionID))
	})
}

const memberEntrySelect = `SELECT me.member_id, m.name, me.competition_id, me.entry, me.updated_at, COALESCE(e.sent_at, '')
	FROM member_entries me
	JOIN members m ON m.id = me.member_id
	LEFT JOIN entries e ON e.member_id = me.member_id AND e.competition_id = me.competition_id`

// memberEntries runs memberEntrySelect with a condition.
func (s *Store) memberEntries(ctx context.Context, where string, args ...any) ([]MemberEntry, error) {
	rows, err := s.db.QueryContext(ctx, memberEntrySelect+" WHERE "+where+" ORDER BY m.name, me.competition_id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemberEntry
	for rows.Next() {
		var e MemberEntry
		var entry, updated, sent string
		if err := rows.Scan(&e.MemberID, &e.MemberName, &e.CompetitionID, &entry, &updated, &sent); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(entry), &e.Entry); err != nil {
			return nil, fmt.Errorf("reading %s's entry: %w", e.MemberName, err)
		}
		e.UpdatedAt, e.SentAt = parseTime(updated), parseTime(sent)
		out = append(out, e)
	}
	return out, rows.Err()
}

// MemberEntries are a member's entries, one per competition.
func (s *Store) MemberEntries(ctx context.Context, memberID string) ([]MemberEntry, error) {
	return s.memberEntries(ctx, "me.member_id = $1", memberID)
}

// ClubEntries are a club's members' entries for a competition, by member.
func (s *Store) ClubEntries(ctx context.Context, clubID, competitionID string) ([]MemberEntry, error) {
	return s.memberEntries(ctx, "m.club_id = $1 AND me.competition_id = $2", clubID, competitionID)
}

// Send sends a club's members' entries to a competition, until its deadline:
// the competition keeps a copy of each as it is now. memberIDs chooses whose
// (nil for everyone's); sending everyone's also takes back entries the club
// sent for members who have since withdrawn or been removed. It returns how
// many entries were sent.
func (s *Store) Send(ctx context.Context, clubID, competitionID string, memberIDs []string) (int, error) {
	sent := 0
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var name string
		err := tx.QueryRowContext(ctx, `SELECT c.name FROM clubs c
			JOIN club_competitions cc ON cc.club_id = c.id AND cc.competition_id = $2
			WHERE c.id = $1`, clubID, competitionID).Scan(&name)
		if err != nil {
			if err == sql.ErrNoRows {
				return ErrNotAttached
			}
			return err
		}
		if err := s.open(ctx, tx, competitionID); err != nil {
			return err
		}

		type pending struct{ member, name, entry string }
		rows, err := tx.QueryContext(ctx, `SELECT me.member_id, m.name, me.entry FROM member_entries me
			JOIN members m ON m.id = me.member_id
			WHERE m.club_id = $1 AND me.competition_id = $2`, clubID, competitionID)
		if err != nil {
			return err
		}
		var all []pending
		for rows.Next() {
			var p pending
			if err := rows.Scan(&p.member, &p.name, &p.entry); err != nil {
				rows.Close()
				return err
			}
			all = append(all, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		chosen := all
		if memberIDs != nil {
			want := map[string]bool{}
			for _, id := range memberIDs {
				want[id] = true
			}
			chosen = nil
			for _, p := range all {
				if want[p.member] {
					chosen = append(chosen, p)
				}
			}
		} else {
			// Everyone's: what the competition has from this club becomes exactly this.
			if _, err := tx.ExecContext(ctx, `DELETE FROM entries WHERE competition_id = $1 AND club_id = $2
				AND (member_id IS NULL OR member_id NOT IN (
					SELECT me.member_id FROM member_entries me JOIN members m ON m.id = me.member_id
					WHERE m.club_id = $2 AND me.competition_id = $1))`, competitionID, clubID); err != nil {
				return err
			}
		}

		now := s.stamp()
		for _, p := range chosen {
			if _, err := tx.ExecContext(ctx, `INSERT INTO entries (id, competition_id, club_id, member_id, club_name, individual, gymnast, entry, sent_at)
				VALUES ($1, $2, $3, $4, $5, FALSE, $6, $7, $8)
				ON CONFLICT (competition_id, member_id) DO UPDATE
				SET entry = excluded.entry, gymnast = excluded.gymnast, club_name = excluded.club_name, sent_at = excluded.sent_at`,
				newID(), competitionID, clubID, p.member, name, p.name, p.entry, now); err != nil {
				return err
			}
		}
		if err := full(ctx, tx, competitionID); err != nil {
			return err
		}
		sent = len(chosen)
		return touch(ctx, tx, clubID, now)
	})
	return sent, err
}
