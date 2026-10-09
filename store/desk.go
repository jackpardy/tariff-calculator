package store

import (
	"context"
	"encoding/json"
	"slices"
	"time"
)

// Messages from the organisers' desk (roadmap 2026-10-09): what the
// organiser sent, or asked someone to come to the desk about, and to whom.

// MaxDeskMessages is how many messages a competition can keep.
const MaxDeskMessages = 500

// DeskMessage is one message to some of a competition's people.
type DeskMessage struct {
	ID       string
	Text     string   // what the organiser wrote
	Come     bool     // it asks them to come to the organisers' desk
	Audience string   // who it went to, in words, e.g. "UCD" or "3 people"
	People   []string // the person keys it went to
	Clubs    []string // the club ids it went to, and all their members
	Staff    []string // the club ids whose comp sec and coaches it went to, not their members
	Who      string   // the link that sent it
	At       time.Time
	Pushed   int // addresses told by push
	Emailed  int // addresses told by email
}

// SendDeskMessage keeps a message, returning it with its id; ErrLimit once the
// competition has MaxDeskMessages.
func (s *Store) SendDeskMessage(ctx context.Context, competitionID string, m DeskMessage) (DeskMessage, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM desk_messages WHERE competition_id = $1`, competitionID).Scan(&n); err != nil {
		return DeskMessage{}, err
	}
	if n >= MaxDeskMessages {
		return DeskMessage{}, ErrLimit
	}
	people, err := json.Marshal(append([]string{}, m.People...))
	if err != nil {
		return DeskMessage{}, err
	}
	clubs, err := json.Marshal(append([]string{}, m.Clubs...))
	if err != nil {
		return DeskMessage{}, err
	}
	staff, err := json.Marshal(append([]string{}, m.Staff...))
	if err != nil {
		return DeskMessage{}, err
	}
	m.ID, m.At = newID(), parseTime(s.stamp())
	_, err = s.db.ExecContext(ctx, `INSERT INTO desk_messages (id, competition_id, text, come, audience, people, clubs, staff, who, at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		m.ID, competitionID, m.Text, m.Come, m.Audience, string(people), string(clubs), string(staff), m.Who, s.stamp())
	return m, err
}

// SetDeskTold records how many addresses a message was pushed and emailed to.
func (s *Store) SetDeskTold(ctx context.Context, competitionID, id string, pushed, emailed int) error {
	return affected(s.db.ExecContext(ctx, `UPDATE desk_messages SET pushed = $1, emailed = $2 WHERE competition_id = $3 AND id = $4`, pushed, emailed, competitionID, id))
}

// DeskMessages are a competition's messages, latest first.
func (s *Store) DeskMessages(ctx context.Context, competitionID string) ([]DeskMessage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, text, come, audience, people, clubs, staff, who, at, pushed, emailed FROM desk_messages WHERE competition_id = $1 ORDER BY at DESC, rowid DESC LIMIT $2`,
		competitionID, MaxDeskMessages)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeskMessage
	for rows.Next() {
		var m DeskMessage
		var people, clubs, staff, at string
		if err := rows.Scan(&m.ID, &m.Text, &m.Come, &m.Audience, &people, &clubs, &staff, &m.Who, &at, &m.Pushed, &m.Emailed); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(people), &m.People)
		json.Unmarshal([]byte(clubs), &m.Clubs)
		json.Unmarshal([]byte(staff), &m.Staff)
		m.At = parseTime(at)
		out = append(out, m)
	}
	return out, rows.Err()
}

// ComeToTheDesk is what a message asking someone to come to the desk starts with.
const ComeToTheDesk = "Please come to the organisers' desk."

// Full is the message as told: the request to come to the desk, if any, then
// what the organiser wrote.
func (m DeskMessage) Full() string {
	switch {
	case !m.Come:
		return m.Text
	case m.Text == "":
		return ComeToTheDesk
	}
	return ComeToTheDesk + " " + m.Text
}

// Reaches says whether the message went to any of these people, to any of
// these clubs (and so their members), or to the comp sec and coaches of any of
// these staff clubs.
func (m DeskMessage) Reaches(people, clubs, staff []string) bool {
	return slices.ContainsFunc(people, func(k string) bool { return slices.Contains(m.People, k) }) ||
		slices.ContainsFunc(clubs, func(c string) bool { return slices.Contains(m.Clubs, c) }) ||
		slices.ContainsFunc(staff, func(c string) bool { return slices.Contains(m.Staff, c) })
}
