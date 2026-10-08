// Package store keeps competitions, clubs, members and their entries in one
// SQLite file (ADR 0004). Access is by secret links, not accounts: each role's
// token is 128 random bits, and only its SHA-256 hash is used to find it, so a
// copied database gives nobody access. Queries are plain database/sql, in SQL
// that Postgres also accepts.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var (
	// ErrNotFound is a link that doesn't lead anywhere: wrong, replaced or
	// deleted. A wrong token and a missing one look the same.
	ErrNotFound = errors.New("not found")
	// ErrClosed is a change after the competition's deadline.
	ErrClosed = errors.New("entries for this competition have closed")
	// ErrNotOpen is a change before the competition goes live, or while
	// the organiser has paused it.
	ErrNotOpen = errors.New("entries for this competition aren't open")
	// ErrLimit is a club or competition that's full.
	ErrLimit = errors.New("limit reached")
	// ErrRemoved is an entry the organiser has removed: it can't be sent or
	// changed until they restore it.
	ErrRemoved = errors.New("the organiser has removed this entry")
	// ErrNotAttached is an entry for a competition the member's club hasn't joined.
	ErrNotAttached = errors.New("the club isn't entered in this competition")
)

const (
	// MaxMembers is the most members a club can have.
	MaxMembers = 300
	// MaxEntries is the most entries a competition can hold.
	MaxEntries = 2000
	// timeLayout keeps times in UTC with a fixed width, so they sort as text.
	timeLayout = "2006-01-02T15:04:05.000000Z"
)

// Store is the database.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open opens (creating if needed) the database in dir and brings its schema up
// to date.
func Open(ctx context.Context, dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating the data directory: %w", err)
	}
	dsn := "file:" + filepath.Join(dir, "tariff.db") +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening the database: %w", err)
	}
	s := &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// migrations upgrade the schema, in order. Each runs once, in a transaction;
// never edit one that has shipped, add another.
var migrations = []string{
	`CREATE TABLE competitions (
		id               TEXT PRIMARY KEY,
		admin_hash       TEXT NOT NULL UNIQUE,
		club_token       TEXT NOT NULL,
		club_hash        TEXT NOT NULL UNIQUE,
		individual_token TEXT NOT NULL,
		individual_hash  TEXT NOT NULL UNIQUE,
		name             TEXT NOT NULL,
		date             TEXT NOT NULL,
		deadline         TEXT NOT NULL,
		individuals      BOOLEAN NOT NULL,
		levels           TEXT NOT NULL,
		created_at       TEXT NOT NULL,
		delete_after     TEXT NOT NULL
	);
	CREATE TABLE clubs (
		id         TEXT PRIMARY KEY,
		admin_hash TEXT NOT NULL UNIQUE,
		join_token TEXT NOT NULL,
		join_hash  TEXT NOT NULL UNIQUE,
		name       TEXT NOT NULL,
		created_at TEXT NOT NULL,
		last_used  TEXT NOT NULL
	);
	CREATE TABLE members (
		id         TEXT PRIMARY KEY,
		club_id    TEXT NOT NULL REFERENCES clubs (id) ON DELETE CASCADE,
		token_hash TEXT NOT NULL UNIQUE,
		name       TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE INDEX members_club ON members (club_id);
	CREATE TABLE club_competitions (
		club_id        TEXT NOT NULL REFERENCES clubs (id) ON DELETE CASCADE,
		competition_id TEXT NOT NULL REFERENCES competitions (id) ON DELETE CASCADE,
		attached_at    TEXT NOT NULL,
		PRIMARY KEY (club_id, competition_id)
	);
	CREATE INDEX club_competitions_competition ON club_competitions (competition_id);
	CREATE TABLE member_entries (
		member_id      TEXT NOT NULL REFERENCES members (id) ON DELETE CASCADE,
		competition_id TEXT NOT NULL REFERENCES competitions (id) ON DELETE CASCADE,
		entry          TEXT NOT NULL,
		updated_at     TEXT NOT NULL,
		PRIMARY KEY (member_id, competition_id)
	);
	CREATE INDEX member_entries_competition ON member_entries (competition_id);
	CREATE TABLE entries (
		id             TEXT PRIMARY KEY,
		competition_id TEXT NOT NULL REFERENCES competitions (id) ON DELETE CASCADE,
		club_id        TEXT REFERENCES clubs (id) ON DELETE SET NULL,
		member_id      TEXT REFERENCES members (id) ON DELETE SET NULL,
		club_name      TEXT NOT NULL,
		individual     BOOLEAN NOT NULL,
		gymnast        TEXT NOT NULL,
		token_hash     TEXT UNIQUE,
		entry          TEXT NOT NULL,
		sent_at        TEXT NOT NULL,
		UNIQUE (competition_id, member_id)
	);
	CREATE INDEX entries_club ON entries (club_id);`,

	// 2: the organiser marks entries checked, with a note back to the club or gymnast.
	`ALTER TABLE entries ADD COLUMN checked_at TEXT;
	ALTER TABLE entries ADD COLUMN note TEXT NOT NULL DEFAULT '';`,

	// 3: video proof (ADR 0004 Decision 10): what each competition asks for, and
	// the organiser's review of each entry's videos.
	`ALTER TABLE competitions ADD COLUMN video TEXT NOT NULL DEFAULT '{}';
	ALTER TABLE entries ADD COLUMN video_review TEXT NOT NULL DEFAULT '';
	ALTER TABLE entries ADD COLUMN video_note TEXT NOT NULL DEFAULT '';`,

	// 4: coaches sign off their members' routines (ADR 0004 Decision 11).
	`CREATE TABLE coaches (
		id         TEXT PRIMARY KEY,
		club_id    TEXT NOT NULL REFERENCES clubs (id) ON DELETE CASCADE,
		token_hash TEXT NOT NULL UNIQUE,
		name       TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE INDEX coaches_club ON coaches (club_id);
	ALTER TABLE clubs ADD COLUMN coaches_see_all BOOLEAN NOT NULL DEFAULT FALSE;
	ALTER TABLE members ADD COLUMN coach_id TEXT REFERENCES coaches (id) ON DELETE SET NULL;
	ALTER TABLE member_entries ADD COLUMN signed_at TEXT;
	ALTER TABLE member_entries ADD COLUMN signed_by TEXT NOT NULL DEFAULT '';
	ALTER TABLE member_entries ADD COLUMN sign_note TEXT NOT NULL DEFAULT '';
	ALTER TABLE competitions ADD COLUMN signoff BOOLEAN NOT NULL DEFAULT FALSE;
	ALTER TABLE entries ADD COLUMN signed_at TEXT;
	ALTER TABLE entries ADD COLUMN signed_by TEXT NOT NULL DEFAULT '';
	ALTER TABLE entries ADD COLUMN sign_note TEXT NOT NULL DEFAULT '';
	ALTER TABLE entries ADD COLUMN signoff_token TEXT NOT NULL DEFAULT '';
	ALTER TABLE entries ADD COLUMN signoff_hash TEXT;
	CREATE UNIQUE INDEX entries_signoff ON entries (signoff_hash);`,

	// 5: the timetable (roadmap: competitions 3), and which levels split men and women.
	`ALTER TABLE competitions ADD COLUMN split TEXT NOT NULL DEFAULT '{}';
	ALTER TABLE competitions ADD COLUMN timetable TEXT NOT NULL DEFAULT '';`,

	// 6: events across disciplines (ADR 0005 Decisions 1-3): a member keeps an
	// entry per discipline, the competition a copy of each, and a synchro
	// entry a partner link. SQLite can't change a key, so both tables are made
	// again and their rows copied.
	`CREATE TABLE member_entries_new (
		member_id         TEXT NOT NULL REFERENCES members (id) ON DELETE CASCADE,
		competition_id    TEXT NOT NULL REFERENCES competitions (id) ON DELETE CASCADE,
		discipline        TEXT NOT NULL DEFAULT '',
		entry             TEXT NOT NULL,
		updated_at        TEXT NOT NULL,
		signed_at         TEXT,
		signed_by         TEXT NOT NULL DEFAULT '',
		sign_note         TEXT NOT NULL DEFAULT '',
		partner_token     TEXT NOT NULL DEFAULT '',
		partner_hash      TEXT,
		partner_confirmed BOOLEAN NOT NULL DEFAULT FALSE,
		partner_member    TEXT,
		partner_entry     TEXT,
		PRIMARY KEY (member_id, competition_id, discipline)
	);
	INSERT INTO member_entries_new (member_id, competition_id, entry, updated_at, signed_at, signed_by, sign_note)
		SELECT member_id, competition_id, entry, updated_at, signed_at, signed_by, sign_note FROM member_entries;
	DROP TABLE member_entries;
	ALTER TABLE member_entries_new RENAME TO member_entries;
	CREATE INDEX member_entries_competition ON member_entries (competition_id);
	CREATE UNIQUE INDEX member_entries_partner ON member_entries (partner_hash);

	CREATE TABLE entries_new (
		id                TEXT PRIMARY KEY,
		competition_id    TEXT NOT NULL REFERENCES competitions (id) ON DELETE CASCADE,
		club_id           TEXT REFERENCES clubs (id) ON DELETE SET NULL,
		member_id         TEXT REFERENCES members (id) ON DELETE SET NULL,
		discipline        TEXT NOT NULL DEFAULT '',
		club_name         TEXT NOT NULL,
		individual        BOOLEAN NOT NULL,
		gymnast           TEXT NOT NULL,
		token_hash        TEXT UNIQUE,
		entry             TEXT NOT NULL,
		sent_at           TEXT NOT NULL,
		checked_at        TEXT,
		note              TEXT NOT NULL DEFAULT '',
		video_review      TEXT NOT NULL DEFAULT '',
		video_note        TEXT NOT NULL DEFAULT '',
		signed_at         TEXT,
		signed_by         TEXT NOT NULL DEFAULT '',
		sign_note         TEXT NOT NULL DEFAULT '',
		signoff_token     TEXT NOT NULL DEFAULT '',
		signoff_hash      TEXT,
		partner_token     TEXT NOT NULL DEFAULT '',
		partner_hash      TEXT,
		partner_confirmed BOOLEAN NOT NULL DEFAULT FALSE,
		partner_member    TEXT,
		partner_entry     TEXT,
		UNIQUE (competition_id, member_id, discipline)
	);
	INSERT INTO entries_new (id, competition_id, club_id, member_id, club_name, individual, gymnast, token_hash, entry, sent_at,
		checked_at, note, video_review, video_note, signed_at, signed_by, sign_note, signoff_token, signoff_hash)
		SELECT id, competition_id, club_id, member_id, club_name, individual, gymnast, token_hash, entry, sent_at,
		checked_at, note, video_review, video_note, signed_at, signed_by, sign_note, signoff_token, signoff_hash FROM entries;
	DROP TABLE entries;
	ALTER TABLE entries_new RENAME TO entries;
	CREATE INDEX entries_club ON entries (club_id);
	CREATE UNIQUE INDEX entries_signoff ON entries (signoff_hash);
	CREATE UNIQUE INDEX entries_partner ON entries (partner_hash);
	ALTER TABLE competitions ADD COLUMN events TEXT NOT NULL DEFAULT '{}';`,

	// 7: officials (ADR 0005 Decisions 5 and 13): what members offer to judge
	// and help, the competition's officials (sent by clubs, offered by
	// individuals, added by the organiser), and its panels and judging rule.
	`CREATE TABLE member_offers (
		member_id      TEXT NOT NULL REFERENCES members (id) ON DELETE CASCADE,
		competition_id TEXT NOT NULL REFERENCES competitions (id) ON DELETE CASCADE,
		offer          TEXT NOT NULL,
		updated_at     TEXT NOT NULL,
		PRIMARY KEY (member_id, competition_id)
	);
	CREATE TABLE officials (
		id             TEXT PRIMARY KEY,
		competition_id TEXT NOT NULL REFERENCES competitions (id) ON DELETE CASCADE,
		club_id        TEXT REFERENCES clubs (id) ON DELETE SET NULL,
		member_id      TEXT REFERENCES members (id) ON DELETE SET NULL,
		entry_id       TEXT REFERENCES entries (id) ON DELETE CASCADE,
		club_name      TEXT NOT NULL DEFAULT '',
		name           TEXT NOT NULL,
		offer          TEXT NOT NULL,
		qualified      BOOLEAN NOT NULL DEFAULT FALSE,
		added          BOOLEAN NOT NULL DEFAULT FALSE,
		updated_at     TEXT NOT NULL,
		UNIQUE (competition_id, member_id),
		UNIQUE (competition_id, entry_id)
	);
	CREATE INDEX officials_competition ON officials (competition_id);
	ALTER TABLE competitions ADD COLUMN officials TEXT NOT NULL DEFAULT '{}';`,

	// 8: levels in order, easiest first. Competitions from before kept the
	// order the form sent, so they're put in order when read
	// (competitions.OrderLevels) until the organiser moves a level.
	`ALTER TABLE competitions ADD COLUMN levels_ordered BOOLEAN NOT NULL DEFAULT FALSE;`,

	// 9: simulated timetables (ADR 0005 Decision 11), kept apart from the
	// real one so planning it never touches them.
	`ALTER TABLE competitions ADD COLUMN scenarios TEXT NOT NULL DEFAULT '[]';`,

	// 10: which coaches sign off, and coaches' qualifications with their
	// certificates, kept with the coach (ADR 0007 Decisions 3, 4 and 9).
	`ALTER TABLE coaches ADD COLUMN signs_off BOOLEAN NOT NULL DEFAULT TRUE;
	CREATE TABLE coach_qualifications (
		id            TEXT PRIMARY KEY,
		coach_id      TEXT NOT NULL REFERENCES coaches (id) ON DELETE CASCADE,
		qualification TEXT NOT NULL,
		cert_type     TEXT NOT NULL,
		certificate   BLOB NOT NULL,
		uploaded_at   TEXT NOT NULL
	);
	CREATE INDEX coach_qualifications_coach ON coach_qualifications (coach_id);`,

	// 11: approving coaches (ADR 0007 Decisions 2, 5, 6 and 7): the
	// competition's setting, which coach signed off, and the coaches clubs
	// send, with the organiser's decision. A sign-off counts from counts_from
	// ('' for all of them).
	`ALTER TABLE competitions ADD COLUMN approve_coaches BOOLEAN NOT NULL DEFAULT FALSE;
	ALTER TABLE competitions ADD COLUMN coach_levels TEXT NOT NULL DEFAULT '{}';
	ALTER TABLE member_entries ADD COLUMN signed_coach TEXT NOT NULL DEFAULT '';
	ALTER TABLE entries ADD COLUMN signed_coach TEXT NOT NULL DEFAULT '';
	CREATE TABLE competition_coaches (
		competition_id TEXT NOT NULL REFERENCES competitions (id) ON DELETE CASCADE,
		coach_id       TEXT NOT NULL REFERENCES coaches (id) ON DELETE CASCADE,
		club_id        TEXT NOT NULL,
		name           TEXT NOT NULL,
		qualifications TEXT NOT NULL DEFAULT '[]',
		sent_at        TEXT NOT NULL,
		status         TEXT NOT NULL DEFAULT '',
		note           TEXT NOT NULL DEFAULT '',
		decided_at     TEXT,
		counts_from    TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (competition_id, coach_id)
	);
	CREATE INDEX competition_coaches_club ON competition_coaches (club_id, competition_id);`,

	// 12: an individual's coach, named on their entry with a qualification
	// and certificate, and the organiser's decision (ADR 0007 Decision 8).
	// It goes with the entry, and so the competition.
	`CREATE TABLE entry_coaches (
		entry_id      TEXT PRIMARY KEY REFERENCES entries (id) ON DELETE CASCADE,
		name          TEXT NOT NULL,
		qualification TEXT NOT NULL,
		cert_type     TEXT NOT NULL,
		certificate   BLOB NOT NULL,
		sent_at       TEXT NOT NULL,
		status        TEXT NOT NULL DEFAULT '',
		note          TEXT NOT NULL DEFAULT '',
		decided_at    TEXT,
		counts_from   TEXT NOT NULL DEFAULT ''
	);`,

	// 13: the organiser removes entries, or holds them for changes (roadmap
	// 2026-10-08), with an optional reason for the club, the member or both;
	// a held entry sent again waits for the organiser (resent).
	`ALTER TABLE entries ADD COLUMN removal TEXT NOT NULL DEFAULT '';
	ALTER TABLE entries ADD COLUMN removal_note TEXT NOT NULL DEFAULT '';
	ALTER TABLE entries ADD COLUMN removal_to TEXT NOT NULL DEFAULT '';
	ALTER TABLE entries ADD COLUMN resent BOOLEAN NOT NULL DEFAULT FALSE;`,

	// 14: draft and published timetables (roadmap 2026-10-07): the organiser
	// changes the draft (timetable), attendees see the published copy. One
	// published before this is its own published copy.
	`ALTER TABLE competitions ADD COLUMN published_timetable TEXT NOT NULL DEFAULT '';
	UPDATE competitions SET published_timetable = timetable WHERE timetable <> '' AND json_extract(timetable, '$.published');`,

	// 15: set up in private, go live when ready (roadmap 2026-10-07): when
	// entries open, '' while private. Those made before this went live
	// when they were made.
	`ALTER TABLE competitions ADD COLUMN live_at TEXT NOT NULL DEFAULT '';
	UPDATE competitions SET live_at = created_at;`,

	// 16: notifications (ADR 0008): who opted in, per competition, and each
	// competition's baseline of what was live and when telling is due.
	`CREATE TABLE notify_subscriptions (
		id             TEXT PRIMARY KEY,
		competition_id TEXT NOT NULL REFERENCES competitions (id) ON DELETE CASCADE,
		kind           TEXT NOT NULL,
		owner_id       TEXT NOT NULL,
		channel        TEXT NOT NULL,
		address        TEXT NOT NULL,
		keys           TEXT NOT NULL DEFAULT '',
		page           TEXT NOT NULL,
		topics         TEXT NOT NULL,
		token          TEXT NOT NULL UNIQUE,
		confirmed      BOOLEAN NOT NULL DEFAULT FALSE,
		created_at     TEXT NOT NULL
	);
	CREATE INDEX notify_subscriptions_owner ON notify_subscriptions (competition_id, kind, owner_id);
	ALTER TABLE competitions ADD COLUMN notify_baseline TEXT NOT NULL DEFAULT '';
	ALTER TABLE competitions ADD COLUMN notify_due TEXT NOT NULL DEFAULT '';
	ALTER TABLE competitions ADD COLUMN notify_no_wait BOOLEAN NOT NULL DEFAULT FALSE;`,

	// 17: the app's own settings, such as its push (VAPID) keys (ADR 0008
	// Decision 3).
	`CREATE TABLE settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`,

	// 18: sign-offs from before migration 11 kept only the coach's name.
	// Where that name is exactly one of the club's coaches, it's that coach,
	// so their sign-offs count once a competition approves them (as asked,
	// 2026-10-08).
	`UPDATE member_entries SET signed_coach = (
		SELECT c.id FROM coaches c JOIN members m ON m.club_id = c.club_id
		WHERE m.id = member_entries.member_id AND lower(trim(c.name)) = lower(trim(member_entries.signed_by)))
	WHERE signed_coach = '' AND signed_by <> '' AND (
		SELECT COUNT(*) FROM coaches c JOIN members m ON m.club_id = c.club_id
		WHERE m.id = member_entries.member_id AND lower(trim(c.name)) = lower(trim(member_entries.signed_by))) = 1;
	UPDATE entries SET signed_coach = (
		SELECT c.id FROM coaches c WHERE c.club_id = entries.club_id AND lower(trim(c.name)) = lower(trim(entries.signed_by)))
	WHERE signed_coach = '' AND signed_by <> '' AND NOT individual AND (
		SELECT COUNT(*) FROM coaches c WHERE c.club_id = entries.club_id AND lower(trim(c.name)) = lower(trim(entries.signed_by))) = 1;`,

	// 19: more admin links, each able to do less, and the change history
	// (ADR 0009).
	`CREATE TABLE competition_links (
		id             TEXT PRIMARY KEY,
		competition_id TEXT NOT NULL REFERENCES competitions (id) ON DELETE CASCADE,
		token_hash     TEXT NOT NULL UNIQUE,
		name           TEXT NOT NULL,
		kind           TEXT NOT NULL,
		created_at     TEXT NOT NULL
	);
	CREATE INDEX competition_links_competition ON competition_links (competition_id);
	CREATE TABLE competition_log (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		competition_id TEXT NOT NULL REFERENCES competitions (id) ON DELETE CASCADE,
		at             TEXT NOT NULL,
		who            TEXT NOT NULL,
		what           TEXT NOT NULL
	);
	CREATE INDEX competition_log_competition ON competition_log (competition_id, id);
	CREATE TABLE concerns (
		id             TEXT PRIMARY KEY,
		competition_id TEXT NOT NULL REFERENCES competitions (id) ON DELETE CASCADE,
		entry_id       TEXT NOT NULL DEFAULT '',
		who            TEXT NOT NULL,
		text           TEXT NOT NULL,
		at             TEXT NOT NULL,
		resolved_at    TEXT NOT NULL DEFAULT '',
		resolved_by    TEXT NOT NULL DEFAULT '',
		resolution     TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX concerns_competition ON concerns (competition_id);`,

	// 20: entry fees (roadmap 2026-10-08): what the competition charges, and
	// payments the organiser records, from a club or an individual's entry.
	`ALTER TABLE competitions ADD COLUMN fees TEXT NOT NULL DEFAULT '{}';
	CREATE TABLE payments (
		id             TEXT PRIMARY KEY,
		competition_id TEXT NOT NULL REFERENCES competitions (id) ON DELETE CASCADE,
		payer          TEXT NOT NULL,
		amount         INTEGER NOT NULL,
		note           TEXT NOT NULL DEFAULT '',
		at             TEXT NOT NULL,
		who            TEXT NOT NULL
	);
	CREATE INDEX payments_competition ON payments (competition_id, payer);`,

	// 21: limits per event with a waiting list (roadmap 2026-10-08): each
	// competition's limits, when each entry first came in (kept through
	// sending again, unless its level changes), and those the organiser lets
	// in over the limit.
	`ALTER TABLE competitions ADD COLUMN limits TEXT NOT NULL DEFAULT '{}';
	ALTER TABLE entries ADD COLUMN entered_at TEXT NOT NULL DEFAULT '';
	UPDATE entries SET entered_at = sent_at;
	ALTER TABLE entries ADD COLUMN let_in BOOLEAN NOT NULL DEFAULT FALSE;`,

	// 22: late changes (roadmap 2026-10-08): which the organiser allows, at
	// what fee, and each request, with the organiser's decision and the fee
	// charged.
	`ALTER TABLE competitions ADD COLUMN late TEXT NOT NULL DEFAULT '{}';
	CREATE TABLE late_requests (
		id             TEXT PRIMARY KEY,
		competition_id TEXT NOT NULL REFERENCES competitions (id) ON DELETE CASCADE,
		entry_id       TEXT NOT NULL,
		kind           TEXT NOT NULL,
		entry          TEXT NOT NULL,
		who            TEXT NOT NULL,
		note           TEXT NOT NULL DEFAULT '',
		at             TEXT NOT NULL,
		status         TEXT NOT NULL DEFAULT '',
		reason         TEXT NOT NULL DEFAULT '',
		decided_at     TEXT NOT NULL DEFAULT '',
		decided_by     TEXT NOT NULL DEFAULT '',
		fee            INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX late_requests_competition ON late_requests (competition_id, entry_id);`,
}

// migrate runs the migrations the database hasn't had yet.
func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("creating the migrations table: %w", err)
	}
	var done int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM migrations`).Scan(&done); err != nil {
		return fmt.Errorf("reading the schema version: %w", err)
	}
	for v := done + 1; v <= len(migrations); v++ {
		err := s.tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, migrations[v-1]); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO migrations (version, applied_at) VALUES ($1, $2)`, v, s.stamp())
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %d: %w", v, err)
		}
	}
	return nil
}

// tx runs fn in a transaction, committing if it returns nil.
func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// stamp is the time now, as stored.
func (s *Store) stamp() string {
	return s.now().UTC().Format(timeLayout)
}

// parseTime reads a stored time.
func parseTime(v string) time.Time {
	t, _ := time.Parse(timeLayout, v)
	return t
}

// formatTime is a time as stored, "" for none.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(timeLayout)
}

// newToken is a secret link token: 128 bits from crypto/rand, as base64url.
func newToken() string {
	b := make([]byte, 16)
	rand.Read(b) // never returns an error (crypto/rand, Go 1.24+)
	return base64.RawURLEncoding.EncodeToString(b)
}

// newID is a row's id: random, so ids say nothing about how many there are.
func newID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// hash is how a token is stored and looked up. Finding a row by its token's
// hash leaks nothing useful through timing: the hash can't be worked back to a
// token.
func hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// affected turns an update or delete that touched nothing into ErrNotFound.
func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// notFound turns sql.ErrNoRows into ErrNotFound.
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
