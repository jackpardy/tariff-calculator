package store

import (
	"context"
	"errors"
	"time"
)

// More admin links, each able to do less, and the change history (ADR
// 0009). The admin link is the organiser's; each extra link has a name,
// who it's for, and a kind, what it can do.

// What an admin link can do.
const (
	LinkOrganiser  = "organiser"  // everything: the admin link
	LinkEverything = "everything" // everything but managing links, replacing the admin link and deleting
	LinkCards      = "cards"      // see the entries, check cards and review videos
	LinkTimetable  = "timetable"  // the timetable and officials, and see the entries
	LinkChair      = "chair"      // chairs of judges: see the entries and print the timetable's sheets
)

// MaxLinks is how many extra links a competition can have.
const MaxLinks = 20

// Link is an admin link: the organiser's, or an extra one.
type Link struct {
	ID        string // "" for the organiser's
	Name      string // who it's for, e.g. "Difficulty judges"; "Organiser" for the admin link
	Kind      string
	CreatedAt time.Time
}

// organiser is the admin link, as a Link.
var organiser = Link{Name: "Organiser", Kind: LinkOrganiser}

// AdminLink is the competition an admin link opens, with which link it is:
// the organiser's or an extra one.
func (s *Store) AdminLink(ctx context.Context, token string) (Competition, Link, error) {
	c, err := s.CompetitionByAdmin(ctx, token)
	if err == nil {
		return c, organiser, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Competition{}, Link{}, err
	}
	var l Link
	var competitionID, created string
	if err := s.db.QueryRowContext(ctx, `SELECT id, competition_id, name, kind, created_at FROM competition_links WHERE token_hash = $1`, hash(token)).
		Scan(&l.ID, &competitionID, &l.Name, &l.Kind, &created); err != nil {
		return Competition{}, Link{}, notFound(err)
	}
	l.CreatedAt = parseTime(created)
	c, err = s.Competition(ctx, competitionID)
	return c, l, err
}

// AddLink makes an extra admin link, returning it and its token, which is
// kept only as a hash: shown once, like the admin link.
func (s *Store) AddLink(ctx context.Context, competitionID, name, kind string) (Link, string, error) {
	switch kind {
	case LinkEverything, LinkCards, LinkTimetable, LinkChair:
	default:
		return Link{}, "", errors.New("unknown kind of link")
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM competition_links WHERE competition_id = $1`, competitionID).Scan(&n); err != nil {
		return Link{}, "", err
	}
	if n >= MaxLinks {
		return Link{}, "", ErrLimit
	}
	l, token := Link{ID: newID(), Name: name, Kind: kind, CreatedAt: parseTime(s.stamp())}, newToken()
	_, err := s.db.ExecContext(ctx, `INSERT INTO competition_links (id, competition_id, token_hash, name, kind, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		l.ID, competitionID, hash(token), name, kind, s.stamp())
	if err != nil {
		return Link{}, "", err
	}
	return l, token, nil
}

// Links are a competition's extra admin links, oldest first.
func (s *Store) Links(ctx context.Context, competitionID string) ([]Link, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, kind, created_at FROM competition_links WHERE competition_id = $1 ORDER BY created_at, rowid`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Link
	for rows.Next() {
		var l Link
		var created string
		if err := rows.Scan(&l.ID, &l.Name, &l.Kind, &created); err != nil {
			return nil, err
		}
		l.CreatedAt = parseTime(created)
		out = append(out, l)
	}
	return out, rows.Err()
}

// RemoveLink stops an extra admin link working.
func (s *Store) RemoveLink(ctx context.Context, competitionID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM competition_links WHERE competition_id = $1 AND id = $2`, competitionID, id))
}

// Change is one change in a competition's history.
type Change struct {
	At        time.Time
	Who, What string
}

// MaxHistory is how many changes a competition's history keeps.
const MaxHistory = 2000

// Record adds a change to a competition's history, keeping the latest
// MaxHistory.
func (s *Store) Record(ctx context.Context, competitionID, who, what string) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO competition_log (competition_id, at, who, what) VALUES ($1, $2, $3, $4)`,
		competitionID, s.stamp(), who, what); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM competition_log WHERE competition_id = $1 AND id <= (
		SELECT id FROM competition_log WHERE competition_id = $1 ORDER BY id DESC LIMIT 1 OFFSET $2)`, competitionID, MaxHistory)
	return err
}

// History is a competition's changes, latest first, at most limit.
func (s *Store) History(ctx context.Context, competitionID string, limit int) ([]Change, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT at, who, what FROM competition_log WHERE competition_id = $1 ORDER BY id DESC LIMIT $2`, competitionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Change
	for rows.Next() {
		var c Change
		var at string
		if err := rows.Scan(&at, &c.Who, &c.What); err != nil {
			return nil, err
		}
		c.At = parseTime(at)
		out = append(out, c)
	}
	return out, rows.Err()
}

// Concern is something a chair of judges (or anyone with an admin link)
// flags for the organiser: about an entry, or the competition generally.
type Concern struct {
	ID         string
	EntryID    string // "" for one about the competition
	Who, Text  string
	At         time.Time
	ResolvedAt time.Time // zero while it's open
	ResolvedBy string
	Resolution string // the note it was resolved with, if any
}

// Open says whether it's still to be dealt with.
func (c Concern) Open() bool { return c.ResolvedAt.IsZero() }

// MaxConcerns is how many concerns a competition can have.
const MaxConcerns = 500

// FlagConcern records a concern.
func (s *Store) FlagConcern(ctx context.Context, competitionID, entryID, who, text string) (Concern, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM concerns WHERE competition_id = $1`, competitionID).Scan(&n); err != nil {
		return Concern{}, err
	}
	if n >= MaxConcerns {
		return Concern{}, ErrLimit
	}
	c := Concern{ID: newID(), EntryID: entryID, Who: who, Text: text, At: parseTime(s.stamp())}
	_, err := s.db.ExecContext(ctx, `INSERT INTO concerns (id, competition_id, entry_id, who, text, at) VALUES ($1, $2, $3, $4, $5, $6)`,
		c.ID, competitionID, entryID, who, text, s.stamp())
	return c, err
}

// Concerns are a competition's concerns, open ones first, each latest first.
func (s *Store) Concerns(ctx context.Context, competitionID string) ([]Concern, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, entry_id, who, text, at, resolved_at, resolved_by, resolution FROM concerns
		WHERE competition_id = $1 ORDER BY resolved_at <> '', at DESC, rowid DESC`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Concern
	for rows.Next() {
		var c Concern
		var at, resolved string
		if err := rows.Scan(&c.ID, &c.EntryID, &c.Who, &c.Text, &at, &resolved, &c.ResolvedBy, &c.Resolution); err != nil {
			return nil, err
		}
		c.At, c.ResolvedAt = parseTime(at), parseTime(resolved)
		out = append(out, c)
	}
	return out, rows.Err()
}

// ResolveConcern marks a concern dealt with, with an optional note.
func (s *Store) ResolveConcern(ctx context.Context, competitionID, id, who, note string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE concerns SET resolved_at = $1, resolved_by = $2, resolution = $3
		WHERE competition_id = $4 AND id = $5 AND resolved_at = ''`, s.stamp(), who, note, competitionID, id))
}
