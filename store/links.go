package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
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
// their own link (migration 48): about an entry, several (and whole events,
// migration 49), or the competition generally. The organiser can say
// they've received a message, and both they and its sender add to it, a
// thread; the resolution is the organiser's own note.
type Concern struct {
	ID         string
	EntryID    string   // the entry a concern flagged from an admin link is about; "" for none
	Entries    []string // the entries a message is about, by id
	Events     []string // the events a message is about, e.g. "BUCS L3"
	Who, Text  string   // who: the admin link's name, or the sender's
	Role       string   // the sender's: RoleMember, RoleCoach, RoleClub or RoleIndividual; "" for a flag from an admin link
	Club       string   // the sender's club's name when they sent it; "" for an individual
	Sender     string   // the sender's record, as Sender.ID; "" for a flag from an admin link
	At         time.Time
	AckAt      time.Time // when the organiser said they'd received it; zero until then
	AckBy      string
	Thread     []Post    // what each side added after it, oldest first
	ResolvedAt time.Time // zero while it's open
	ResolvedBy string
	Resolution string // the organiser's own note, from resolving it
}

// Post is something added to a message after it was sent: the organiser's
// reply, or the sender's.
type Post struct {
	ID, Who, Text string
	BySender      bool // the sender's; otherwise the organiser's (Who is the link's name)
	At            time.Time
}

// Open says whether it's still to be dealt with.
func (c Concern) Open() bool { return c.ResolvedAt.IsZero() }

// Acknowledged says whether the organiser has said they've received it (or
// replied, or resolved it).
func (c Concern) Acknowledged() bool {
	return !c.AckAt.IsZero() || !c.Open() || slices.ContainsFunc(c.Thread, func(p Post) bool { return !p.BySender })
}

// Waiting says whether the last word is the sender's: the message, or what
// they added since the organiser's last reply.
func (c Concern) Waiting() bool {
	return c.Message() && (len(c.Thread) == 0 || c.Thread[len(c.Thread)-1].BySender)
}

// Message says whether it's a message from someone's own link, which they
// see again with the organiser's replies, rather than a concern flagged
// from an admin link.
func (c Concern) Message() bool { return c.Role != "" }

// Sender is who sends the organiser a message, from their own link.
type Sender struct {
	Role string // RoleMember, RoleCoach, RoleClub or RoleIndividual
	Name string // their name; the club's for a comp sec
	Club string // their club's name; "" for an individual
	ID   string // their record, as kind:id: how many they send, and which are theirs
}

// About is what a message is about: entries, by id, and whole events; the
// competition generally with neither.
type About struct {
	Entries, Events []string
}

const (
	// MaxConcerns is how many concerns and messages a competition can have.
	MaxConcerns = 2000
	// MaxMessagesPerDay is how many messages one sender can send a
	// competition's organiser in 24 hours.
	MaxMessagesPerDay = 10
	// MaxPosts is how much both sides can add to a message.
	MaxPosts = 50
)

// FlagConcern records a concern from an admin link.
func (s *Store) FlagConcern(ctx context.Context, competitionID, entryID, who, text string) (Concern, error) {
	return s.addConcern(ctx, competitionID, Concern{EntryID: entryID, Who: who, Text: text})
}

// TellOrganiser records a message to the organiser from someone's own
// link, refusing it with ErrLimit once they've sent MaxMessagesPerDay in the
// last 24 hours.
func (s *Store) TellOrganiser(ctx context.Context, competitionID string, about About, from Sender, text string) (Concern, error) {
	var n int
	since := formatTime(s.now().Add(-24 * time.Hour))
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM concerns WHERE competition_id = $1 AND sender = $2 AND at > $3`,
		competitionID, from.ID, since).Scan(&n); err != nil {
		return Concern{}, err
	}
	if n >= MaxMessagesPerDay {
		return Concern{}, ErrLimit
	}
	return s.addConcern(ctx, competitionID, Concern{Entries: about.Entries, Events: about.Events, Who: from.Name, Role: from.Role, Club: from.Club, Sender: from.ID, Text: text})
}

// addConcern keeps a concern or message, up to MaxConcerns a competition.
func (s *Store) addConcern(ctx context.Context, competitionID string, c Concern) (Concern, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM concerns WHERE competition_id = $1`, competitionID).Scan(&n); err != nil {
		return Concern{}, err
	}
	if n >= MaxConcerns {
		return Concern{}, ErrLimit
	}
	entries, err := jsonList(c.Entries)
	if err != nil {
		return Concern{}, err
	}
	events, err := jsonList(c.Events)
	if err != nil {
		return Concern{}, err
	}
	at := s.stamp()
	c.ID, c.At = newID(), parseTime(at)
	_, err = s.db.ExecContext(ctx, `INSERT INTO concerns (id, competition_id, entry_id, who, text, at, role, club, sender, entries, events)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		c.ID, competitionID, c.EntryID, c.Who, c.Text, at, c.Role, c.Club, c.Sender, entries, events)
	return c, err
}

// jsonList is a list of strings as kept: [] for none.
func jsonList(list []string) (string, error) {
	if len(list) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(list)
	return string(b), err
}

// SetMessages lets people send the competition's organiser messages, or
// stops them.
func (s *Store) SetMessages(ctx context.Context, competitionID string, on bool) error {
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET messages = $1 WHERE id = $2`, on, competitionID))
}

// Concerns are a competition's concerns and messages, open ones first, each
// latest first, with their threads.
func (s *Store) Concerns(ctx context.Context, competitionID string) ([]Concern, error) {
	return s.concerns(ctx, `competition_id = $1`, competitionID)
}

// SentBy are the messages one sender (Sender.ID) has sent a competition's
// organiser, latest first, with their threads.
func (s *Store) SentBy(ctx context.Context, competitionID, sender string) ([]Concern, error) {
	if sender == "" {
		return nil, nil
	}
	all, err := s.concerns(ctx, `competition_id = $1 AND sender = $2`, competitionID, sender)
	slices.SortStableFunc(all, func(a, b Concern) int { return b.At.Compare(a.At) })
	return all, err
}

// concerns are the concerns where (on concerns' own columns), open ones
// first, each latest first, with their threads.
func (s *Store) concerns(ctx context.Context, where string, args ...any) ([]Concern, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, entry_id, entries, events, who, role, club, sender, text, at, acknowledged_at, acknowledged_by,
		resolved_at, resolved_by, resolution FROM concerns
		WHERE `+where+` ORDER BY resolved_at <> '', at DESC, rowid DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Concern
	byID := map[string]int{}
	for rows.Next() {
		var c Concern
		var entries, events, at, ack, resolved string
		if err := rows.Scan(&c.ID, &c.EntryID, &entries, &events, &c.Who, &c.Role, &c.Club, &c.Sender, &c.Text, &at, &ack, &c.AckBy,
			&resolved, &c.ResolvedBy, &c.Resolution); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(entries), &c.Entries)
		json.Unmarshal([]byte(events), &c.Events)
		c.At, c.AckAt, c.ResolvedAt = parseTime(at), parseTime(ack), parseTime(resolved)
		byID[c.ID] = len(out)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	posts, err := s.db.QueryContext(ctx, `SELECT concern_id, id, by_sender, who, text, at FROM concern_posts
		WHERE concern_id IN (SELECT id FROM concerns WHERE `+where+`) ORDER BY at, rowid`, args...)
	if err != nil {
		return nil, err
	}
	defer posts.Close()
	for posts.Next() {
		var concernID, at string
		var p Post
		if err := posts.Scan(&concernID, &p.ID, &p.BySender, &p.Who, &p.Text, &at); err != nil {
			return nil, err
		}
		p.At = parseTime(at)
		if i, ok := byID[concernID]; ok {
			out[i].Thread = append(out[i].Thread, p)
		}
	}
	return out, posts.Err()
}

// Concern is one of a competition's concerns or messages, with its thread.
func (s *Store) Concern(ctx context.Context, competitionID, id string) (Concern, error) {
	all, err := s.concerns(ctx, `competition_id = $1 AND id = $2`, competitionID, id)
	if err != nil {
		return Concern{}, err
	}
	if len(all) == 0 {
		return Concern{}, ErrNotFound
	}
	return all[0], nil
}

// Acknowledge says the organiser has received a message: ErrNotFound if
// it isn't there, or has been acknowledged or resolved already.
func (s *Store) Acknowledge(ctx context.Context, competitionID, id, who string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE concerns SET acknowledged_at = $1, acknowledged_by = $2
		WHERE competition_id = $3 AND id = $4 AND acknowledged_at = '' AND resolved_at = ''`, s.stamp(), who, competitionID, id))
}

// ReplyTo adds the organiser's reply (by who, the link's name) to a
// message, which says it's received too; ErrLimit past MaxPosts,
// ErrNotFound for a concern that isn't a message.
func (s *Store) ReplyTo(ctx context.Context, competitionID, id, who, text string) (Post, error) {
	return s.post(ctx, competitionID, id, "", Post{Who: who, Text: text})
}

// AddToMessage adds the sender's own words to a message they sent
// (sender, as Sender.ID), opening it again if it was resolved;
// ErrNotFound if it isn't theirs, ErrLimit past MaxPosts.
func (s *Store) AddToMessage(ctx context.Context, competitionID, id, sender, text string) (Post, error) {
	if sender == "" {
		return Post{}, ErrNotFound
	}
	return s.post(ctx, competitionID, id, sender, Post{BySender: true, Text: text})
}

// post adds to a message's thread: the sender's (sender set, which must be
// the message's), or the organiser's.
func (s *Store) post(ctx context.Context, competitionID, id, sender string, p Post) (Post, error) {
	p.ID = newID()
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		var who, from string
		if err := tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM concern_posts WHERE concern_id = c.id), c.who, c.sender FROM concerns c
			WHERE c.competition_id = $1 AND c.id = $2 AND c.role <> ''`, competitionID, id).Scan(&n, &who, &from); err != nil {
			return notFound(err)
		}
		if sender != "" && sender != from {
			return ErrNotFound
		}
		if n >= MaxPosts {
			return ErrLimit
		}
		at := s.stamp()
		p.At = parseTime(at)
		if p.BySender {
			p.Who = who
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO concern_posts (id, concern_id, at, by_sender, who, text) VALUES ($1, $2, $3, $4, $5, $6)`,
			p.ID, id, at, p.BySender, p.Who, p.Text); err != nil {
			return err
		}
		if p.BySender {
			// More from them: it's open again, for the organiser to see.
			_, err := tx.ExecContext(ctx, `UPDATE concerns SET resolved_at = '', resolved_by = '' WHERE id = $1`, id)
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE concerns SET acknowledged_at = $1, acknowledged_by = $2 WHERE id = $3 AND acknowledged_at = ''`, at, p.Who, id)
		return err
	})
	return p, err
}

// ResolveConcern marks a concern dealt with, with an optional note of the
// organiser's own.
func (s *Store) ResolveConcern(ctx context.Context, competitionID, id, who, note string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE concerns SET resolved_at = $1, resolved_by = $2, resolution = $3
		WHERE competition_id = $4 AND id = $5 AND resolved_at = ''`, s.stamp(), who, note, competitionID, id))
}
