package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"tariffCalculator/competitions"
)

// Copying a competition (2026-11-13): its set-up for another competition,
// or with its entries as a rehearsal copy only its own admin link opens;
// and a finished competition's set-up kept once its people are deleted
// (ADR 0004, the note of 2026-11-13).

// How a competition was made (Competition.MadeAs).
const (
	MadeNew       = ""
	MadeCopy      = "copy"
	MadeRehearsal = "rehearsal"
)

const (
	// RehearsalDays is how long a rehearsal copy is kept after it's made,
	// and how much longer once extended.
	RehearsalDays = 30
	// SetupKeptYears is how long a kept set-up is kept after the
	// competition's date.
	SetupKeptYears = 3
)

// ErrExtended is a rehearsal copy kept longer already: once only.
var ErrExtended = errors.New("this rehearsal copy has been kept longer already")

// Rehearsal says whether it's a rehearsal copy.
func (c Competition) Rehearsal() bool { return c.MadeAs == MadeRehearsal }

// SetupKept says whether its people were deleted, keeping its set-up.
func (c Competition) SetupKept() bool { return !c.SetupKeptAt.IsZero() }

// setupColumns are the competition's set-up, copied as they are: the rest
// of a copy's row is its own (copyRow).
var setupColumns = []string{"individuals", "levels", "video", "signoff", "split", "events", "officials", "levels_ordered",
	"approve_coaches", "coach_levels", "notify_no_wait", "fees", "limits", "late", "messages", "clubs_send"}

// CopyCompetition copies a competition as a new one, named name, on date
// (its first day): its set-up alone, or as a rehearsal copy with its
// entries and everything about the people in them, in one transaction. A
// copy starts private, without deadlines. It returns the copy with its
// admin link.
func (s *Store) CopyCompetition(ctx context.Context, fromID, name, date string, rehearsal bool) (Competition, string, error) {
	admin := newToken()
	id := newID()
	err := s.tx(ctx, func(tx *sql.Tx) error {
		from, err := scanCompetition(tx.QueryRowContext(ctx, `SELECT `+competitionColumns+` FROM competitions WHERE id = $1`, fromID))
		if err != nil {
			return err
		}
		if rehearsal && (from.Rehearsal() || from.SetupKept()) {
			return errors.New("only a competition with its entries can be copied for a rehearsal")
		}
		var c *copier
		if rehearsal {
			if c, err = s.readPeople(ctx, tx, fromID, id); err != nil {
				return err
			}
		}
		if err := s.copyRow(ctx, tx, from, id, admin, name, date, c); err != nil {
			return err
		}
		if c != nil {
			return c.write(ctx, tx)
		}
		return nil
	})
	if err != nil {
		return Competition{}, "", err
	}
	out, err := s.Competition(ctx, id)
	return out, admin, err
}

// copyRow writes the copy's row of competitions: the set-up as it is, a
// new name, date and links, no deadlines, private, and its timetable's
// Setup moved to the new date; a rehearsal copy (c) also has the timetables
// and what-ifs, its people's ids changed for the copy's.
func (s *Store) copyRow(ctx context.Context, tx *sql.Tx, from Competition, id, admin, name, date string, c *copier) error {
	day, err := time.Parse(competitions.DateLayout, date)
	if err != nil {
		return fmt.Errorf("the copy's date %q: %w", date, err)
	}
	was, _ := from.Day()
	clubLink, individualLink := newToken(), newToken()
	clubHash, individualHash := hash(clubLink), hash(individualLink)
	madeAs, deletes := MadeCopy, competitions.Competition{Date: date}.DeleteAfter()
	var timetable, published, scenarios string
	if c == nil {
		if from.Timetable != nil {
			// Its Setup alone, without the officials' own rules: they're
			// not copied.
			setup := setupOnly(from.Timetable.Setup)
			moveDays(&setup, was, day)
			data, err := json.Marshal(competitions.Schedule{Setup: setup})
			if err != nil {
				return err
			}
			timetable = string(data)
		}
		scenarios = "[]"
	} else {
		// Nobody enters it: links that open nothing.
		clubHash, individualHash = deadHash(), deadHash()
		madeAs, deletes = MadeRehearsal, parseTime(s.stamp()).AddDate(0, 0, RehearsalDays)
		if err := tx.QueryRowContext(ctx, `SELECT timetable, published_timetable, scenarios FROM competitions WHERE id = $1`, from.ID).
			Scan(&timetable, &published, &scenarios); err != nil {
			return err
		}
		for _, t := range []*string{&timetable, &published} {
			if *t == "" {
				continue
			}
			var sc competitions.Schedule
			if err := json.Unmarshal([]byte(*t), &sc); err != nil {
				return err
			}
			moveDays(&sc.Setup, was, day)
			data, err := json.Marshal(sc)
			if err != nil {
				return err
			}
			*t = c.ids.Replace(string(data))
		}
	}
	cols := strings.Join(setupColumns, ", ")
	_, err = tx.ExecContext(ctx, `INSERT INTO competitions (id, admin_hash, club_token, club_hash, individual_token, individual_hash, name, date, deadline,
			created_at, delete_after, timetable, published_timetable, scenarios, made_as, copied_from, `+cols+`)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, '', $9, $10, $11, $12, $13, $14, name, `+cols+` FROM competitions WHERE id = $15`,
		id, hash(admin), clubLink, clubHash, individualLink, individualHash, name, date, s.stamp(), formatTime(deletes),
		timetable, published, scenarios, madeAs, from.ID)
	return err
}

// setupOnly is a timetable's Setup without the rules about officials, who
// are people.
func setupOnly(setup competitions.Setup) competitions.Setup {
	var rules []competitions.Rule
	for _, r := range setup.Rules {
		if r.Person == "" {
			rules = append(rules, r)
		}
	}
	setup.Rules = rules
	return setup
}

// moveDays renames a Setup's days named for their weekdays from was (the
// first day) for the weekdays from day: "Saturday" stays the first day's
// name.
func moveDays(setup *competitions.Setup, was, day time.Time) {
	if was.IsZero() {
		return
	}
	for i := range setup.Days {
		if setup.Days[i].Name == was.AddDate(0, 0, i).Weekday().String() {
			setup.Days[i].Name = day.AddDate(0, 0, i).Weekday().String()
		}
	}
}

// deadHash is a link's hash no link has: a hash is hex, this isn't.
func deadHash() string { return "none:" + newID() }

// copier is a rehearsal copy's people, read from the competition copied,
// with a new id for every id among them (ids), to write as the copy's.
type copier struct {
	rehearsal string // the copy's id
	now       string
	tables    []copiedTable
	ids       *strings.Replacer
}

// copiedTable is one table's rows to copy.
type copiedTable struct {
	name    string
	columns []string
	rows    [][]any
}

// peopleTables are the tables a rehearsal copy has rows of, in an order
// that keeps every reference good, each with the rows of the competition
// copied ($1): its clubs, coaches and members as far as it sees them (those
// its entries, offers and coaches sent name), its individuals, entries,
// offers, officials, coaches sent, judges' and coaches' qualifications and
// payments. Not copied: helper links, history, messages and concerns, late
// requests, what was marked on the day, notifications and feedback.
var peopleTables = []struct{ table, where string }{
	{"clubs", `id IN (` + copiedClubs + `)`},
	{"coaches", `id IN (` + copiedCoaches + `)`},
	{"members", `id IN (` + copiedMembers + `)`},
	{"coach_qualifications", `coach_id IN (` + copiedCoaches + `)`},
	{"individuals", `id IN (SELECT individual_id FROM entries WHERE competition_id = $1)`},
	{"club_competitions", `competition_id = $1`},
	{"entries", `competition_id = $1`},
	{"member_entries", `competition_id = $1`},
	{"member_offers", `competition_id = $1`},
	{"officials", `competition_id = $1`},
	{"judge_qualifications", `(member_id IS NULL OR member_id IN (` + copiedMembers + `))
		AND (entry_id IN (SELECT id FROM entries WHERE competition_id = $1) OR (entry_id IS NULL AND member_id IN (` + copiedMembers + `)))`},
	{"competition_coaches", `competition_id = $1`},
	{"entry_coaches", `entry_id IN (SELECT id FROM entries WHERE competition_id = $1)`},
	{"payments", `competition_id = $1`},
}

// The members, coaches and clubs a competition sees: those its entries,
// offers, officials and coaches sent name, with their members' coaches and
// everyone's clubs.
const (
	copiedMembers = `SELECT member_id FROM entries WHERE competition_id = $1
		UNION SELECT partner_member FROM entries WHERE competition_id = $1
		UNION SELECT member_id FROM member_entries WHERE competition_id = $1
		UNION SELECT partner_member FROM member_entries WHERE competition_id = $1
		UNION SELECT member_id FROM member_offers WHERE competition_id = $1
		UNION SELECT member_id FROM officials WHERE competition_id = $1`
	copiedCoaches = `SELECT coach_id FROM members WHERE id IN (` + copiedMembers + `)
		UNION SELECT coach_id FROM competition_coaches WHERE competition_id = $1
		UNION SELECT signed_coach FROM entries WHERE competition_id = $1
		UNION SELECT signed_coach FROM member_entries WHERE competition_id = $1`
	copiedClubs = `SELECT club_id FROM club_competitions WHERE competition_id = $1
		UNION SELECT club_id FROM entries WHERE competition_id = $1
		UNION SELECT club_id FROM officials WHERE competition_id = $1
		UNION SELECT club_id FROM competition_coaches WHERE competition_id = $1
		UNION SELECT club_id FROM members WHERE id IN (` + copiedMembers + `)
		UNION SELECT club_id FROM coaches WHERE id IN (` + copiedCoaches + `)`
)

// idColumns are the columns holding a row's id or another's: each value
// gets a new id in the copy, wherever it appears (person keys, payers and
// the timetables hold them too).
var idColumns = map[string]bool{"id": true, "competition_id": true, "club_id": true, "member_id": true, "coach_id": true,
	"entry_id": true, "individual_id": true, "person_id": true, "partner_member": true, "partner_entry": true, "signed_coach": true}

// Link columns: a copy's hashes open nothing and its tokens are new, so no
// link to the competition copied, or anyone in it, opens anything in the
// copy.
var (
	hashColumns  = map[string]bool{"admin_hash": true, "join_hash": true, "token_hash": true, "signoff_hash": true, "partner_hash": true}
	tokenColumns = map[string]bool{"join_token": true, "signoff_token": true, "partner_token": true}
)

// readPeople reads the people of competition fromID to copy as rehearsal's.
func (s *Store) readPeople(ctx context.Context, tx *sql.Tx, fromID, rehearsal string) (*copier, error) {
	c := &copier{rehearsal: rehearsal, now: s.stamp()}
	ids := map[string]string{fromID: rehearsal}
	for _, t := range peopleTables {
		rows, err := tx.QueryContext(ctx, `SELECT * FROM `+t.table+` WHERE `+t.where, fromID)
		if err != nil {
			return nil, err
		}
		ct := copiedTable{name: t.table}
		if ct.columns, err = rows.Columns(); err != nil {
			rows.Close()
			return nil, err
		}
		for rows.Next() {
			row := make([]any, len(ct.columns))
			ptrs := make([]any, len(row))
			for i := range row {
				ptrs[i] = &row[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return nil, err
			}
			for i, col := range ct.columns {
				if v, ok := row[i].(string); ok && v != "" && idColumns[col] && ids[v] == "" {
					ids[v] = newID()
				}
			}
			ct.rows = append(ct.rows, row)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		c.tables = append(c.tables, ct)
	}
	pairs := make([]string, 0, 2*len(ids))
	for from, to := range ids {
		pairs = append(pairs, from, to)
	}
	c.ids = strings.NewReplacer(pairs...)
	return c, nil
}

// write writes the people read as the rehearsal copy's: every id the copy's
// own, links that open nothing, and its clubs and individuals marked as
// the copy's, to go with it.
func (c *copier) write(ctx context.Context, tx *sql.Tx) error {
	for _, t := range c.tables {
		if len(t.rows) == 0 {
			continue
		}
		columns := t.columns
		marks := make([]string, len(columns))
		for i := range marks {
			marks[i] = fmt.Sprintf("$%d", i+1)
		}
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO `+t.name+` (`+strings.Join(columns, ", ")+`) VALUES (`+strings.Join(marks, ", ")+`)`)
		if err != nil {
			return err
		}
		for _, row := range t.rows {
			values := make([]any, 0, len(columns))
			for i, col := range t.columns {
				v := row[i]
				switch s, ok := v.(string); {
				case col == "rehearsal_id":
					v = c.rehearsal // its clubs and individuals go with it
				case !ok || s == "":
				case hashColumns[col]:
					v = deadHash()
				case tokenColumns[col]:
					v = newToken()
				case col == "last_used":
					v = c.now // a club of the copy's isn't deleted as unused
				default:
					v = c.ids.Replace(s)
				}
				values = append(values, v)
			}
			if _, err := stmt.ExecContext(ctx, values...); err != nil {
				stmt.Close()
				return fmt.Errorf("copying %s: %w", t.name, err)
			}
		}
		stmt.Close()
	}
	return nil
}

// ExtendRehearsal keeps a rehearsal copy RehearsalDays longer, once.
func (s *Store) ExtendRehearsal(ctx context.Context, id string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var deletes string
		var extended bool
		if err := tx.QueryRowContext(ctx, `SELECT delete_after, extended FROM competitions WHERE id = $1 AND made_as = 'rehearsal'`, id).
			Scan(&deletes, &extended); err != nil {
			return notFound(err)
		}
		if extended {
			return ErrExtended
		}
		_, err := tx.ExecContext(ctx, `UPDATE competitions SET delete_after = $1, extended = TRUE WHERE id = $2`,
			formatTime(parseTime(deletes).AddDate(0, 0, RehearsalDays)), id)
		return err
	})
}

// competitionTables are the tables of a competition's own rows, all deleted
// when its set-up is kept: everything about the people in it.
var competitionTables = []string{"entries", "member_entries", "member_offers", "officials", "club_competitions", "competition_coaches",
	"competition_links", "competition_log", "concerns", "late_requests", "payments", "notify_subscriptions", "desk_messages",
	"flight_times", "checkins", "scratch_clears", "official_checkins", "up_now", "screen_notices"}

// keepSetups deletes the people of the competitions where keep (a
// condition on competitions, with args) holds, and everything about them,
// keeping each one's set-up until SetupKeptYears after its date: its
// timetable's Setup without the officials' rules, and its settings without
// the organiser's free text (the payment instructions and the note to
// officials), which could name someone.
func (s *Store) keepSetups(ctx context.Context, tx *sql.Tx, keep string, args ...any) (int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, date, timetable, officials, fees FROM competitions WHERE `+keep, args...)
	if err != nil {
		return 0, err
	}
	type kept struct{ id, date, timetable, officials, fees string }
	var all []kept
	for rows.Next() {
		var k kept
		if err := rows.Scan(&k.id, &k.date, &k.timetable, &k.officials, &k.fees); err != nil {
			rows.Close()
			return 0, err
		}
		all = append(all, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, k := range all {
		for _, table := range competitionTables {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE competition_id = $1`, k.id); err != nil {
				return 0, err
			}
		}
		if k.timetable != "" {
			var sc competitions.Schedule
			if err := json.Unmarshal([]byte(k.timetable), &sc); err != nil {
				return 0, fmt.Errorf("reading competition %s's timetable: %w", k.id, err)
			}
			data, err := json.Marshal(competitions.Schedule{Setup: setupOnly(sc.Setup)})
			if err != nil {
				return 0, err
			}
			k.timetable = string(data)
		}
		var officials competitions.OfficialSettings
		var fees competitions.Fees
		if err := json.Unmarshal([]byte(k.officials), &officials); err != nil {
			return 0, err
		}
		if err := json.Unmarshal([]byte(k.fees), &fees); err != nil {
			return 0, err
		}
		officials.Note, fees.Instructions = "", ""
		o, err := json.Marshal(officials)
		if err != nil {
			return 0, err
		}
		f, err := json.Marshal(fees)
		if err != nil {
			return 0, err
		}
		deletes := time.Time{}
		if day, err := time.Parse(competitions.DateLayout, k.date); err == nil {
			deletes = day.AddDate(SetupKeptYears, 0, 0)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE competitions SET timetable = $2, published_timetable = '', scenarios = '[]',
				officials = $3, fees = $4, notify_baseline = '', notify_due = '',
				club_token = $5, club_hash = $6, individual_token = $7, individual_hash = $8,
				setup_kept_at = $9, delete_after = $10 WHERE id = $1`,
			k.id, k.timetable, string(o), string(f), newToken(), deadHash(), newToken(), deadHash(), s.stamp(), formatTime(deletes)); err != nil {
			return 0, err
		}
	}
	// The admin link on people's own pages goes with them.
	if _, err := tx.ExecContext(ctx, `DELETE FROM shared_ties WHERE kind = 'organiser' AND target_id IN (SELECT id FROM competitions WHERE setup_kept_at <> '')`); err != nil {
		return 0, err
	}
	return int64(len(all)), nil
}
