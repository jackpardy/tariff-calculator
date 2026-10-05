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
	// ErrLimit is a club or competition that's full.
	ErrLimit = errors.New("limit reached")
	// ErrNotAttached is an entry for a competition the member's club hasn't joined.
	ErrNotAttached = errors.New("the club isn't entered in this competition")
)

const (
	// MaxMembers is the most members a club can have.
	MaxMembers = 300
	// MaxEntries is the most entries a competition can hold.
	MaxEntries = 2000
	// timeLayout keeps times in UTC with a fixed width, so they sort as text.
	timeLayout = "2006-01-02T15:04:05Z"
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
