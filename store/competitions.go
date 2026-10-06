package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"tariffCalculator/competitions"
)

// Competition is a stored competition. Its club and individual links are
// shared, so they're kept to show again; its admin link is shown once, when
// it's made or replaced.
type Competition struct {
	ID string
	competitions.Competition
	ClubLink       string // comp secs attach their club and send its entries
	IndividualLink string // individuals enter directly, if Individuals allows
	CreatedAt      time.Time
}

// CreateCompetition stores a validated competition and returns it with its
// admin link.
func (s *Store) CreateCompetition(ctx context.Context, c competitions.Competition) (Competition, string, error) {
	levels, err := json.Marshal(c.Levels)
	if err != nil {
		return Competition{}, "", fmt.Errorf("encoding levels: %w", err)
	}
	out := Competition{ID: newID(), Competition: c, ClubLink: newToken(), IndividualLink: newToken(), CreatedAt: parseTime(s.stamp())}
	admin := newToken()
	video, err := json.Marshal(c.Video)
	if err != nil {
		return Competition{}, "", fmt.Errorf("encoding video: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO competitions
		(id, admin_hash, club_token, club_hash, individual_token, individual_hash, name, date, deadline, individuals, levels, created_at, delete_after, video)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		out.ID, hash(admin), out.ClubLink, hash(out.ClubLink), out.IndividualLink, hash(out.IndividualLink),
		c.Name, c.Date, c.Deadline.UTC().Format(timeLayout), c.Individuals, string(levels),
		out.CreatedAt.Format(timeLayout), c.DeleteAfter().Format(timeLayout), string(video))
	if err != nil {
		return Competition{}, "", fmt.Errorf("storing the competition: %w", err)
	}
	return out, admin, nil
}

const competitionColumns = `id, club_token, individual_token, name, date, deadline, individuals, levels, created_at, video`

// scanCompetition reads a row of competitionColumns.
func scanCompetition(row interface{ Scan(...any) error }) (Competition, error) {
	var c Competition
	var deadline, levels, created, video string
	if err := row.Scan(&c.ID, &c.ClubLink, &c.IndividualLink, &c.Name, &c.Date, &deadline, &c.Individuals, &levels, &created, &video); err != nil {
		return Competition{}, notFound(err)
	}
	if err := json.Unmarshal([]byte(video), &c.Video); err != nil {
		return Competition{}, fmt.Errorf("reading competition %s's video: %w", c.ID, err)
	}
	if err := json.Unmarshal([]byte(levels), &c.Levels); err != nil {
		return Competition{}, fmt.Errorf("reading competition %s's levels: %w", c.ID, err)
	}
	c.Deadline, c.CreatedAt = parseTime(deadline), parseTime(created)
	return c, nil
}

// competitionBy finds a competition by one of its links' hashes.
func (s *Store) competitionBy(ctx context.Context, column, token string) (Competition, error) {
	return scanCompetition(s.db.QueryRowContext(ctx,
		`SELECT `+competitionColumns+` FROM competitions WHERE `+column+` = $1`, hash(token)))
}

// CompetitionByAdmin is the competition an admin link opens.
func (s *Store) CompetitionByAdmin(ctx context.Context, token string) (Competition, error) {
	return s.competitionBy(ctx, "admin_hash", token)
}

// CompetitionByClubLink is the competition a club link enters.
func (s *Store) CompetitionByClubLink(ctx context.Context, token string) (Competition, error) {
	return s.competitionBy(ctx, "club_hash", token)
}

// CompetitionByIndividualLink is the competition an individual link enters,
// while it takes individual entries.
func (s *Store) CompetitionByIndividualLink(ctx context.Context, token string) (Competition, error) {
	c, err := s.competitionBy(ctx, "individual_hash", token)
	if err == nil && !c.Individuals {
		return Competition{}, ErrNotFound
	}
	return c, err
}

// Competition is a competition by id.
func (s *Store) Competition(ctx context.Context, id string) (Competition, error) {
	return scanCompetition(s.db.QueryRowContext(ctx,
		`SELECT `+competitionColumns+` FROM competitions WHERE id = $1`, id))
}

// ReplaceCompetitionAdmin gives a competition a new admin link, which ends the old one.
func (s *Store) ReplaceCompetitionAdmin(ctx context.Context, id string) (string, error) {
	admin := newToken()
	return admin, affected(s.db.ExecContext(ctx, `UPDATE competitions SET admin_hash = $1 WHERE id = $2`, hash(admin), id))
}

// DeleteCompetition deletes a competition and everything entered for it.
func (s *Store) DeleteCompetition(ctx context.Context, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM competitions WHERE id = $1`, id))
}

// Entry is the competition's copy of an entry, as a club sent it or an
// individual entered it.
type Entry struct {
	ID            string
	CompetitionID string
	ClubID        string // "" for an individual, or once the club is deleted
	ClubName      string // "" for an individual
	MemberID      string // the member it was sent for; "" for an individual
	Individual    bool
	Entry         competitions.Entry
	SentAt        time.Time
	CheckedAt     time.Time // when the organiser marked it checked; zero if not, or changed since
	Note          string    // the organiser's note back to the club or gymnast
	VideoReview   string    // the organiser's review of its videos: "", VideoOK or VideoMore
	VideoNote     string    // what more the organiser needs
	// Withdrawn is a club's entry whose member has since withdrawn it, or left
	// the club. It stays until the club sends everyone's again (ADR 0004
	// Decision 2: the organiser only sees what the club sends), marked so it
	// isn't taken for a live entry.
	Withdrawn bool
}

const (
	// VideoOK and VideoMore are the organiser's review of an entry's videos:
	// they show what's needed, or more is needed (VideoNote says what).
	VideoOK   = "ok"
	VideoMore = "more"
)

// Checked says whether the organiser has checked the entry as it is now.
func (e Entry) Checked() bool { return !e.CheckedAt.IsZero() }

// Entries are everything entered for a competition: clubs' entries by club,
// then individuals', each by gymnast.
func (s *Store) Entries(ctx context.Context, competitionID string) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, competition_id, COALESCE(club_id, ''), club_name, COALESCE(member_id, ''), individual, entry, sent_at, COALESCE(checked_at, ''), note, video_review, video_note,
		club_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM member_entries me
			WHERE me.member_id = entries.member_id AND me.competition_id = entries.competition_id)
		FROM entries WHERE competition_id = $1
		ORDER BY individual, club_name, gymnast, id`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// scanEntry reads a row of entries, as Entries selects it.
func scanEntry(row interface{ Scan(...any) error }) (Entry, error) {
	var e Entry
	var entry, sent, checked string
	if err := row.Scan(&e.ID, &e.CompetitionID, &e.ClubID, &e.ClubName, &e.MemberID, &e.Individual, &entry, &sent, &checked, &e.Note, &e.VideoReview, &e.VideoNote, &e.Withdrawn); err != nil {
		return Entry{}, notFound(err)
	}
	if err := json.Unmarshal([]byte(entry), &e.Entry); err != nil {
		return Entry{}, fmt.Errorf("reading entry %s: %w", e.ID, err)
	}
	e.SentAt, e.CheckedAt = parseTime(sent), parseTime(checked)
	return e, nil
}

// open reads a competition's deadline in a transaction, failing with ErrClosed
// once it has passed.
func (s *Store) open(ctx context.Context, tx *sql.Tx, competitionID string) error {
	var deadline string
	if err := tx.QueryRowContext(ctx, `SELECT deadline FROM competitions WHERE id = $1`, competitionID).Scan(&deadline); err != nil {
		return notFound(err)
	}
	if !s.now().Before(parseTime(deadline)) {
		return ErrClosed
	}
	return nil
}

// full fails with ErrLimit when a competition holds more than MaxEntries, so
// the transaction adding them rolls back.
func full(ctx context.Context, tx *sql.Tx, competitionID string) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM entries WHERE competition_id = $1`, competitionID).Scan(&n); err != nil {
		return err
	}
	if n > MaxEntries {
		return ErrLimit
	}
	return nil
}

// AddIndividualEntry enters a validated entry as an individual, and returns
// it with its personal link, used to replace or withdraw it.
func (s *Store) AddIndividualEntry(ctx context.Context, competitionID string, e competitions.Entry) (Entry, string, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return Entry{}, "", err
	}
	token := newToken()
	out := Entry{ID: newID(), CompetitionID: competitionID, Individual: true, Entry: e, SentAt: parseTime(s.stamp())}
	err = s.tx(ctx, func(tx *sql.Tx) error {
		if err := s.open(ctx, tx, competitionID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO entries (id, competition_id, club_name, individual, gymnast, token_hash, entry, sent_at)
			VALUES ($1, $2, '', TRUE, $3, $4, $5, $6)`, out.ID, competitionID, e.Gymnast, hash(token), string(data), out.SentAt.Format(timeLayout)); err != nil {
			return err
		}
		return full(ctx, tx, competitionID)
	})
	if err != nil {
		return Entry{}, "", err
	}
	return out, token, nil
}

// IndividualEntry is the entry an individual's personal link opens.
func (s *Store) IndividualEntry(ctx context.Context, token string) (Entry, error) {
	return scanEntry(s.db.QueryRowContext(ctx, `SELECT id, competition_id, '', club_name, '', individual, entry, sent_at, COALESCE(checked_at, ''), note, video_review, video_note, FALSE
		FROM entries WHERE token_hash = $1`, hash(token)))
}

// ReplaceIndividualEntry replaces an individual's entry, until the deadline.
func (s *Store) ReplaceIndividualEntry(ctx context.Context, token string, e competitions.Entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		var competitionID string
		if err := tx.QueryRowContext(ctx, `SELECT competition_id FROM entries WHERE token_hash = $1`, hash(token)).Scan(&competitionID); err != nil {
			return notFound(err)
		}
		if err := s.open(ctx, tx, competitionID); err != nil {
			return err
		}
		return affected(tx.ExecContext(ctx, `UPDATE entries SET entry = $1, gymnast = $2, sent_at = $3,
			checked_at = CASE WHEN entry = $1 THEN checked_at ELSE NULL END,
			video_review = CASE WHEN entry = $1 THEN video_review ELSE '' END WHERE token_hash = $4`, string(data), e.Gymnast, s.stamp(), hash(token)))
	})
}

// WithdrawIndividualEntry removes an individual's entry, until the deadline.
func (s *Store) WithdrawIndividualEntry(ctx context.Context, token string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var competitionID string
		if err := tx.QueryRowContext(ctx, `SELECT competition_id FROM entries WHERE token_hash = $1`, hash(token)).Scan(&competitionID); err != nil {
			return notFound(err)
		}
		if err := s.open(ctx, tx, competitionID); err != nil {
			return err
		}
		return affected(tx.ExecContext(ctx, `DELETE FROM entries WHERE token_hash = $1`, hash(token)))
	})
}

// DeleteExpired deletes competitions past their retention date, and clubs
// unused for competitions.RetentionDays, with everything they hold. It
// returns how many of each it deleted.
func (s *Store) DeleteExpired(ctx context.Context) (comps, clubs int64, err error) {
	now := s.now()
	err = s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM competitions WHERE delete_after <= $1`, now.Format(timeLayout))
		if err != nil {
			return err
		}
		if comps, err = res.RowsAffected(); err != nil {
			return err
		}
		unused := now.AddDate(0, 0, -competitions.RetentionDays).Format(timeLayout)
		if res, err = tx.ExecContext(ctx, `DELETE FROM clubs WHERE last_used <= $1`, unused); err != nil {
			return err
		}
		clubs, err = res.RowsAffected()
		return err
	})
	return comps, clubs, err
}

// SetIndividuals turns a competition's individual entry on or off.
func (s *Store) SetIndividuals(ctx context.Context, id string, on bool) error {
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET individuals = $1 WHERE id = $2`, on, id))
}

// CompetitionEntry is one of a competition's entries, by id.
func (s *Store) CompetitionEntry(ctx context.Context, competitionID, id string) (Entry, error) {
	return scanEntry(s.db.QueryRowContext(ctx, `SELECT id, competition_id, COALESCE(club_id, ''), club_name, COALESCE(member_id, ''), individual, entry, sent_at, COALESCE(checked_at, ''), note, video_review, video_note,
		club_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM member_entries me
			WHERE me.member_id = entries.member_id AND me.competition_id = entries.competition_id)
		FROM entries WHERE competition_id = $1 AND id = $2`, competitionID, id))
}

// MarkChecked marks an entry checked (or not) with a note for the club or
// gymnast. Changing the entry afterwards unchecks it; the note stays.
func (s *Store) MarkChecked(ctx context.Context, competitionID, id string, checked bool, note string) error {
	var at any
	if checked {
		at = s.stamp()
	}
	return affected(s.db.ExecContext(ctx, `UPDATE entries SET checked_at = $1, note = $2 WHERE competition_id = $3 AND id = $4`,
		at, note, competitionID, id))
}

// SetDeadline changes when a competition's entries close: now to close them,
// later to reopen or extend them.
func (s *Store) SetDeadline(ctx context.Context, id string, deadline time.Time) error {
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET deadline = $1 WHERE id = $2`, deadline.UTC().Format(timeLayout), id))
}

// ReviewVideo records the organiser's review of an entry's videos: VideoOK,
// VideoMore with a note of what's needed, or "" to clear it. Changing the
// entry afterwards clears the review; the note stays.
func (s *Store) ReviewVideo(ctx context.Context, competitionID, id, review, note string) error {
	if review != "" && review != VideoOK && review != VideoMore {
		return fmt.Errorf("unknown video review %q", review)
	}
	return affected(s.db.ExecContext(ctx, `UPDATE entries SET video_review = $1, video_note = $2 WHERE competition_id = $3 AND id = $4`,
		review, note, competitionID, id))
}

// SetVideo changes what video proof a competition asks for.
func (s *Store) SetVideo(ctx context.Context, id string, v competitions.Video) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET video = $1 WHERE id = $2`, string(data), id))
}
