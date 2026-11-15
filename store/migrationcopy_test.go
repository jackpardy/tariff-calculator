package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// copiesIn are the names of the copies in dir.
func copiesIn(t *testing.T, dir string) []string {
	t.Helper()
	files, err := migrationCopies(dir)
	must(t, err)
	var out []string
	for _, f := range files {
		out = append(out, filepath.Base(f.path))
	}
	return out
}

// behind opens a database in dir one migration behind and makes a club in it.
func behind(t *testing.T, dir string, by int) {
	t.Helper()
	all := migrations
	t.Cleanup(func() { migrations = all })
	migrations = all[:len(all)-by]
	s, err := Open(ctx, dir)
	must(t, err)
	if _, _, err := s.CreateClub(ctx, "UCD"); err != nil {
		t.Fatal(err)
	}
	must(t, s.Close())
	migrations = all
}

// A database one migration behind is copied before it's migrated: the copy
// is named for the migration about to run and is the database as it was.
// A new database is copied never, and an up-to-date one isn't either.
func TestCopyBeforeMigrating(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	must(t, err)
	must(t, s.Close())
	if got := copiesIn(t, dir); len(got) != 0 {
		t.Errorf("a new database was copied: %v", got)
	}
	s, err = Open(ctx, dir)
	must(t, err)
	must(t, s.Close())
	if got := copiesIn(t, dir); len(got) != 0 {
		t.Errorf("an up-to-date database was copied: %v", got)
	}

	dir = t.TempDir()
	behind(t, dir, 1)
	s, err = Open(ctx, dir)
	must(t, err)
	defer s.Close()
	want := MigrationCopyName(len(migrations))
	if got := copiesIn(t, dir); len(got) != 1 || got[0] != want {
		t.Fatalf("copies: %v, want [%s]", got, want)
	}
	fi, err := os.Stat(filepath.Join(dir, want))
	must(t, err)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("the copy's permissions: %v", fi.Mode().Perm())
	}
	// It reads as a database at the old version, with what was in it.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, want)+"?mode=ro")
	must(t, err)
	defer db.Close()
	var version, clubs int
	must(t, db.QueryRow(`SELECT MAX(version) FROM migrations`).Scan(&version))
	must(t, db.QueryRow(`SELECT COUNT(*) FROM clubs`).Scan(&clubs))
	if version != len(migrations)-1 || clubs != 1 {
		t.Errorf("the copy is at version %d with %d clubs, want %d and 1", version, clubs, len(migrations)-1)
	}
}

// Only the newest two copies are kept.
func TestCopiesKeptTwo(t *testing.T) {
	dir := t.TempDir()
	all := migrations
	t.Cleanup(func() { migrations = all })
	migrations = all[:len(all)-3]
	s, err := Open(ctx, dir)
	must(t, err)
	must(t, s.Close())
	for n := 2; n >= 0; n-- {
		migrations = all[:len(all)-n]
		s, err := Open(ctx, dir)
		must(t, err)
		must(t, s.Close())
		time.Sleep(20 * time.Millisecond) // so each copy is plainly newer than the last
	}
	want := []string{MigrationCopyName(len(all)), MigrationCopyName(len(all) - 1)}
	if got := copiesIn(t, dir); !slices.Equal(got, want) {
		t.Fatalf("copies: %v, want the newest two, %v", got, want)
	}
}

// With no room, the app doesn't migrate and the database is as it was.
func TestNoRoomNoMigration(t *testing.T) {
	dir := t.TempDir()
	behind(t, dir, 1)
	was := freeSpace
	t.Cleanup(func() { freeSpace = was })
	freeSpace = func(string) (uint64, error) { return 1 << 20, nil }
	if s, err := Open(ctx, dir); err == nil {
		s.Close()
		t.Fatal("opened without room for the copy")
	}
	if got := copiesIn(t, dir); len(got) != 0 {
		t.Errorf("copies: %v", got)
	}
	freeSpace = was
	s, err := Open(ctx, dir)
	must(t, err)
	defer s.Close()
	var n int
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM migrations`).Scan(&n))
	if n != len(migrations) {
		t.Errorf("migrated to %d, want %d once there's room", n, len(migrations))
	}
}

// The expiry job deletes copies older than MigrationCopyDays, and any left
// half written, and no newer.
func TestOldCopiesDeleted(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	must(t, err)
	defer s.Close()
	old := time.Now().AddDate(0, 0, -MigrationCopyDays-1)
	for name, at := range map[string]time.Time{"tariff-before-v1.db": old, "tariff-before-v2.db.tmp": old, "tariff-before-v3.db": time.Now(), "notes.txt": old} {
		must(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600))
		must(t, os.Chtimes(filepath.Join(dir, name), at, at))
	}
	n, err := s.DeleteOldMigrationCopies(time.Now().AddDate(0, 0, -MigrationCopyDays))
	must(t, err)
	if n != 2 {
		t.Errorf("deleted %d, want 2", n)
	}
	if got := copiesIn(t, dir); len(got) != 1 || got[0] != "tariff-before-v3.db" {
		t.Errorf("left: %v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Error("deleted a file that isn't a copy")
	}
}
