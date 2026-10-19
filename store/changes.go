package store

import (
	"bytes"
	"context"
	"database/sql"
	"sync"
	"sync/atomic"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// database is the store's connection pool, counting the changes made
// through it that change what a competition's pages work out from its
// entries (Writes): the pages keep that until the count moves.
//
// Every change goes through it (ExecContext, Store.tx), on a connection
// that watches each row changed (SQLite's pre-update hook, changeSet.note).
// A row of a table the pages don't read from (watched) isn't counted at
// all: check-ins, who's up, desk messages, notices, feedback, push
// subscriptions, marking a club used. A row of a competition's own counts
// for that competition alone; a change to a person, who may be in any
// competition, counts for every one.
type database struct {
	*sql.DB
	all     atomic.Uint64 // changes that count for every competition
	mu      sync.Mutex
	by      map[string]uint64 // changes that count for one, by competition id
	columns map[string]map[string]int
	// Each watched table's columns, by name: their place in its rows,
	// read when the store opens. Until then (while migrating), and for a
	// table without the columns watched names (in a test's older schema),
	// every change to a watched table counts for every competition.
}

// watch is how a change to a row of a watched table counts.
type watch struct {
	// competition is the column holding the competition the row is part
	// of: a change counts for that competition. "" is a row that isn't any
	// one competition's, so a change counts for every one.
	competition string
	// read are the columns the pages read, of a table without competition:
	// changing others (when a club was last used, a name the pages don't
	// show) isn't counted. None: every change is.
	read []string
	// unseen is a table whose new rows the pages can't see until a
	// competition's row refers to them (which counts for that one), so
	// adding one isn't counted.
	unseen bool
}

// watched are the tables a competition's pages work out what they show
// from (Competition, Entries, Officials and PersonGroups read them). Keep it
// in step with those: a table they read that isn't here leaves the pages
// showing what it held before a change. TestWatchedTables names every other
// table, so a new table has to be put in one list or the other.
var watched = map[string]watch{
	"competitions":        {competition: "id"},
	"entries":             {competition: "competition_id"},
	"member_entries":      {competition: "competition_id"},
	"officials":           {competition: "competition_id"},
	"club_competitions":   {competition: "competition_id"},
	"competition_coaches": {competition: "competition_id"},
	"entry_coaches":       {},
	"members":             {read: []string{"id", "club_id", "coach_id", "person_id"}},
	"coaches":             {read: []string{"id", "club_id", "name", "person_id"}},
	"individuals":         {read: []string{"id", "person_id"}, unseen: true},
}

// readColumns reads where each watched table's columns are in its rows.
func (d *database) readColumns(ctx context.Context) error {
	columns := map[string]map[string]int{}
	for table, w := range watched {
		rows, err := d.QueryContext(ctx, `SELECT cid, name FROM pragma_table_info($1)`, table)
		if err != nil {
			return err
		}
		columns[table] = map[string]int{}
		for rows.Next() {
			var cid int
			var name string
			if err := rows.Scan(&cid, &name); err != nil {
				rows.Close()
				return err
			}
			columns[table][name] = cid
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, name := range append([]string{w.competition}, w.read...) {
			if _, ok := columns[table][name]; name != "" && !ok {
				delete(columns, table) // an older schema's: count every change
				break
			}
		}
	}
	d.mu.Lock()
	d.columns = columns
	d.mu.Unlock()
	return nil
}

// Writes counts the changes made so far that may change what a
// competition's pages work out from its entries: it moves after each one is
// saved, so what was read when it stood at a number is out of date once it
// moves. It counts only this process's changes; the app is the database's
// only writer while it runs.
func (s *Store) Writes(competitionID string) uint64 {
	s.db.mu.Lock()
	defer s.db.mu.Unlock()
	return s.db.all.Load() + s.db.by[competitionID]
}

// ExecContext runs a statement, counting what it changed once it's done.
func (d *database) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var res sql.Result
	err := d.watching(ctx, func(conn *sql.Conn) error {
		var err error
		res, err = conn.ExecContext(ctx, query, args...)
		return err
	})
	return res, err
}

// preUpdateHooker is a connection that can watch each row changed.
type preUpdateHooker interface {
	RegisterPreUpdateHook(sqlite.PreUpdateHookFn)
}

// watching runs fn on a connection of its own, watching what it changes,
// and counts that once fn is done: after a transaction it runs has
// committed, so a page reading meanwhile reads again next time. A change
// that's rolled back is counted too, which only reads again what's the
// same.
func (d *database) watching(ctx context.Context, fn func(*sql.Conn) error) error {
	conn, err := d.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	d.mu.Lock()
	ch := &changeSet{columns: d.columns}
	d.mu.Unlock()
	hooked := false
	conn.Raw(func(dc any) error {
		if h, ok := dc.(preUpdateHooker); ok {
			h.RegisterPreUpdateHook(ch.note)
			hooked = true
		}
		return nil
	})
	done := false
	defer func() { // after fn, even if it panics, and before the connection goes back
		if hooked {
			conn.Raw(func(dc any) error {
				dc.(preUpdateHooker).RegisterPreUpdateHook(nil)
				return nil
			})
		}
		if !hooked || !done {
			ch.all = true // can't tell what changed: count it for everyone
		}
		d.count(ch)
	}()
	err = fn(conn)
	done = true
	return err
}

// count counts a change.
func (d *database) count(ch *changeSet) {
	switch {
	case ch.all:
		d.mu.Lock() // so Writes reads both counts as they stood together
		d.all.Add(1)
		d.mu.Unlock()
	case len(ch.competitions) > 0:
		d.mu.Lock()
		if d.by == nil {
			d.by = map[string]uint64{}
		}
		for id := range ch.competitions {
			d.by[id]++
		}
		d.mu.Unlock()
	}
}

// changeSet is what one statement or transaction changed, as far as the
// pages care.
type changeSet struct {
	columns      map[string]map[string]int // database.columns
	all          bool                      // it counts for every competition
	competitions map[string]bool           // the competitions it counts for
}

// note notes a row about to change (SQLite's pre-update hook): inserted,
// deleted or updated, by a statement or by a foreign key's cascade.
func (ch *changeSet) note(u sqlite.SQLitePreUpdateData) {
	w, ok := watched[u.TableName]
	if ch.all || !ok || u.DatabaseName != "main" {
		return
	}
	columns := ch.columns[u.TableName]
	if columns == nil || u.OldRowID != u.NewRowID && u.Op == sqlite3.SQLITE_UPDATE {
		ch.all = true
		return
	}
	if w.competition == "" {
		switch u.Op {
		case sqlite3.SQLITE_INSERT:
			ch.all = !w.unseen
		case sqlite3.SQLITE_UPDATE:
			ch.all = len(w.read) == 0 || ch.readChanged(&u, w.read, columns)
		default:
			ch.all = true
		}
		return
	}
	var sides []func(...any) error
	if u.Op != sqlite3.SQLITE_INSERT {
		sides = append(sides, u.Old)
	}
	if u.Op != sqlite3.SQLITE_DELETE {
		sides = append(sides, u.New)
	}
	n := columns[w.competition] + 1
	for _, side := range sides {
		row := make([]any, n)
		id, ok := "", false
		if side(row...) == nil {
			id, ok = row[n-1].(string)
		}
		if !ok {
			ch.all = true
			return
		}
		if ch.competitions == nil {
			ch.competitions = map[string]bool{}
		}
		ch.competitions[id] = true
	}
}

// readChanged says whether an update changes any of the columns named.
func (ch *changeSet) readChanged(u *sqlite.SQLitePreUpdateData, read []string, columns map[string]int) bool {
	n := 0
	for _, name := range read {
		n = max(n, columns[name]+1)
	}
	old, now := make([]any, n), make([]any, n)
	if u.Old(old...) != nil || u.New(now...) != nil {
		return true
	}
	for _, name := range read {
		a, b := old[columns[name]], now[columns[name]]
		if ab, ok := a.([]byte); ok {
			if bb, ok := b.([]byte); !ok || !bytes.Equal(ab, bb) {
				return true
			}
		} else if _, ok := b.([]byte); ok || a != b {
			return true
		}
	}
	return false
}
