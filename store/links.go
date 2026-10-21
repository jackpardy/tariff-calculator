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
	LinkScreen     = "screen"     // a venue screen: what's on each area now and next, and nothing else
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
// the organiser's or an extra one. A personal link's Via to the competition
// (for the organiser's) or to the extra link opens it while the person has
// that link, as it is now, on their page (ADR 0010).
func (s *Store) AdminLink(ctx context.Context, token string) (Competition, Link, error) {
	if personal, id, ok := splitVia(token); ok {
		return s.adminVia(ctx, personal, id)
	}
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

// adminVia is the admin link a personal link's Via opens: the organiser's,
// by the competition's id, or an extra one, by its id.
func (s *Store) adminVia(ctx context.Context, personal, id string) (Competition, Link, error) {
	h, err := holderOf(ctx, s.db, personal)
	if err != nil {
		return Competition{}, Link{}, err
	}
	var kind, competitionID, created string
	var l Link
	err = s.db.QueryRowContext(ctx, `SELECT 'organiser', c.id, '', '', '', '' FROM shared_ties t JOIN competitions c ON c.id = t.target_id AND c.admin_hash = t.link_hash
			WHERE t.kind = 'organiser' AND t.target_id = $1 AND t.record_id IN (`+personRecords("2")+`)
		UNION ALL SELECT 'helper', l.competition_id, l.id, l.name, l.kind, l.created_at FROM shared_ties t JOIN competition_links l ON l.id = t.target_id AND l.token_hash = t.link_hash
			WHERE t.kind = 'helper' AND t.target_id = $1 AND t.record_id IN (`+personRecords("2")+`)
		LIMIT 1`, id, h.PersonID).Scan(&kind, &competitionID, &l.ID, &l.Name, &l.Kind, &created)
	if err != nil {
		return Competition{}, Link{}, notFound(err)
	}
	c, err := s.Competition(ctx, competitionID)
	if kind == RoleOrganiser {
		return c, organiser, err
	}
	l.CreatedAt = parseTime(created)
	return c, l, err
}

// AddLink makes an extra admin link, returning it and its token, which is
// kept only as a hash: shown once, like the admin link.
func (s *Store) AddLink(ctx context.Context, competitionID, name, kind string) (Link, string, error) {
	switch kind {
	case LinkEverything, LinkCards, LinkTimetable, LinkChair, LinkScreen:
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
// flags for the organiser, or a message someone sends the organiser from
// their own link (migration 48): about an entry, or the competition
// generally.
type Concern struct {
	ID         string
	EntryID    string // "" for one about the competition
	Who, Text  string // who: the admin link's name, or the sender's
	Role       string // the sender's: RoleMember, RoleCoach, RoleClub or RoleIndividual; "" for a flag from an admin link
	Club       string // the sender's club's name when they sent it; "" for an individual
	At         time.Time
	ResolvedAt time.Time // zero while it's open
	ResolvedBy string
	Resolution string // the note it was resolved with, if any
}

// Open says whether it's still to be dealt with.
func (c Concern) Open() bool { return c.ResolvedAt.IsZero() }

// Sender is who sends the organiser a message, from their own link.
type Sender struct {
	Role string // RoleMember, RoleCoach, RoleClub or RoleIndividual
	Name string // their name; the club's for a comp sec
	Club string // their club's name; "" for an individual
	ID   string // their record, as kind:id, for how many they send
}

const (
	// MaxConcerns is how many concerns and messages a competition can have.
	MaxConcerns = 2000
	// MaxMessagesPerDay is how many messages one sender can send a
	// competition's organiser in 24 hours.
	MaxMessagesPerDay = 10
)

// FlagConcern records a concern from an admin link.
func (s *Store) FlagConcern(ctx context.Context, competitionID, entryID, who, text string) (Concern, error) {
	return s.addConcern(ctx, competitionID, Concern{EntryID: entryID, Who: who, Text: text}, "")
}

// TellOrganiser records a message to the organiser from someone's own
// link, refusing it with ErrLimit once they've sent MaxMessagesPerDay in the
// last 24 hours.
func (s *Store) TellOrganiser(ctx context.Context, competitionID, entryID string, from Sender, text string) (Concern, error) {
	var n int
	since := formatTime(s.now().Add(-24 * time.Hour))
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM concerns WHERE competition_id = $1 AND sender = $2 AND at > $3`,
		competitionID, from.ID, since).Scan(&n); err != nil {
		return Concern{}, err
	}
	if n >= MaxMessagesPerDay {
		return Concern{}, ErrLimit
	}
	return s.addConcern(ctx, competitionID, Concern{EntryID: entryID, Who: from.Name, Role: from.Role, Club: from.Club, Text: text}, from.ID)
}

// addConcern keeps a concern or message, up to MaxConcerns a competition.
func (s *Store) addConcern(ctx context.Context, competitionID string, c Concern, sender string) (Concern, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM concerns WHERE competition_id = $1`, competitionID).Scan(&n); err != nil {
		return Concern{}, err
	}
	if n >= MaxConcerns {
		return Concern{}, ErrLimit
	}
	at := s.stamp()
	c.ID, c.At = newID(), parseTime(at)
	_, err := s.db.ExecContext(ctx, `INSERT INTO concerns (id, competition_id, entry_id, who, text, at, role, club, sender) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		c.ID, competitionID, c.EntryID, c.Who, c.Text, at, c.Role, c.Club, sender)
	return c, err
}

// SetMessages lets people send the competition's organiser messages, or
// stops them.
func (s *Store) SetMessages(ctx context.Context, competitionID string, on bool) error {
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET messages = $1 WHERE id = $2`, on, competitionID))
}

// Concerns are a competition's concerns and messages, open ones first, each
// latest first.
func (s *Store) Concerns(ctx context.Context, competitionID string) ([]Concern, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, entry_id, who, role, club, text, at, resolved_at, resolved_by, resolution FROM concerns
		WHERE competition_id = $1 ORDER BY resolved_at <> '', at DESC, rowid DESC`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Concern
	for rows.Next() {
		var c Concern
		var at, resolved string
		if err := rows.Scan(&c.ID, &c.EntryID, &c.Who, &c.Role, &c.Club, &c.Text, &at, &resolved, &c.ResolvedBy, &c.Resolution); err != nil {
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
