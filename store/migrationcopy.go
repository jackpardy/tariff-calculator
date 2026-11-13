package store

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// A copy before migrating: each migration runs in one transaction, so one
// that fails rolls back, but one that succeeds and is wrong could otherwise
// only be undone from last night's backup. Before the first pending
// migration runs (and only on a database that already has data), a
// consistent copy is written beside the database, named for the migration
// about to run (tariff-before-v59.db is the database as it was before
// migration 59). The newest copiesKept stay. A copy holds everything the
// database does, so DeleteOldMigrationCopies deletes it after
// MigrationCopyDays, with what has expired.

const (
	// copiesKept is how many copies are kept.
	copiesKept = 2
	// MigrationCopyDays is how long a copy is kept.
	MigrationCopyDays = 14
	// copyMargin is the space wanted free beyond a copy's size.
	copyMargin = 64 << 20
)

// freeSpace is the space free on dir's disk: a variable so a test can say
// there's none.
var freeSpace = diskFree

// migrationCopy matches a copy's file name, and the temporary name it has
// while being written.
var migrationCopy = regexp.MustCompile(`^tariff-before-v\d+\.db(\.tmp)?$`)

// Dir is the data directory the database and its copies are in.
func (s *Store) Dir() string { return s.dir }

// MigrationCopyName is the file name of the copy made before migration v.
func MigrationCopyName(v int) string { return fmt.Sprintf("tariff-before-v%d.db", v) }

// copyBeforeMigrating writes the copy made before migration v runs, refusing
// if it can't be written or there isn't room for it. A store with no
// directory has nowhere to put one.
func (s *Store) copyBeforeMigrating(ctx context.Context, v int) error {
	if s.dir == "" {
		return nil
	}
	started := time.Now()
	var size int64
	for _, name := range []string{"tariff.db", "tariff.db-wal"} {
		if fi, err := os.Stat(filepath.Join(s.dir, name)); err == nil {
			size += fi.Size()
		}
	}
	// Room for the copy, with a margin: checked as for certificates, only
	// where the free space can be told.
	if free, err := s.free(); err == nil && free < uint64(size+size/10+copyMargin) {
		return fmt.Errorf("there isn't room for a copy of the database (%d MB, with %d MB free)", size>>20, free>>20)
	}
	final := filepath.Join(s.dir, MigrationCopyName(v))
	tmp := final + ".tmp"
	os.Remove(tmp)
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO $1`, tmp); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("writing the copy: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return err
	}
	log.Printf("Copied the database to %s before migrating (%d MB, took %s)", filepath.Base(final), size>>20, time.Since(started).Round(time.Millisecond))
	// Only the newest copiesKept stay.
	files, err := migrationCopies(s.dir)
	if err != nil {
		log.Printf("Listing the database's copies: %v", err)
		return nil
	}
	for i := copiesKept; i < len(files); i++ {
		if err := os.Remove(files[i].path); err != nil {
			log.Printf("Deleting an old copy of the database: %v", err)
		}
	}
	return nil
}

type copyFile struct {
	path string
	at   time.Time
}

// migrationCopies are the copies in dir, newest first.
func migrationCopies(dir string) ([]copyFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []copyFile
	for _, e := range entries {
		if !migrationCopy.MatchString(e.Name()) || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, copyFile{filepath.Join(dir, e.Name()), fi.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].at.After(out[j].at) })
	return out, nil
}

// DeleteOldMigrationCopies deletes the copies of the database made before
// migrating that were written before cutoff, and any left half written, and
// returns how many it deleted: a copy holds what may since have been
// deleted from the database itself.
func (s *Store) DeleteOldMigrationCopies(cutoff time.Time) (int, error) {
	if s.dir == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, e := range entries {
		if !migrationCopy.MatchString(e.Name()) {
			continue
		}
		if fi, err := e.Info(); err != nil || fi.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, e.Name())); err != nil {
			errs = append(errs, err)
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}
