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
	Timetable      *competitions.Schedule // the timetable and its setup; nil until the organiser sets one up
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
	split, err := json.Marshal(c.Split)
	if err != nil {
		return Competition{}, "", fmt.Errorf("encoding split: %w", err)
	}
	events, err := json.Marshal(otherEvents{c.Synchro, c.Tumbling, c.DMT})
	if err != nil {
		return Competition{}, "", fmt.Errorf("encoding events: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO competitions
		(id, admin_hash, club_token, club_hash, individual_token, individual_hash, name, date, deadline, individuals, levels, created_at, delete_after, video, signoff, split, events, levels_ordered)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, TRUE)`,
		out.ID, hash(admin), out.ClubLink, hash(out.ClubLink), out.IndividualLink, hash(out.IndividualLink),
		c.Name, c.Date, c.Deadline.UTC().Format(timeLayout), c.Individuals, string(levels),
		out.CreatedAt.Format(timeLayout), c.DeleteAfter().Format(timeLayout), string(video), c.Signoff, string(split), string(events))
	if err != nil {
		return Competition{}, "", fmt.Errorf("storing the competition: %w", err)
	}
	return out, admin, nil
}

const competitionColumns = `id, club_token, individual_token, name, date, deadline, individuals, levels, created_at, video, signoff, split, timetable, events, officials, levels_ordered, approve_coaches, coach_levels`

// scanCompetition reads a row of competitionColumns.
func scanCompetition(row interface{ Scan(...any) error }) (Competition, error) {
	var c Competition
	var deadline, levels, created, video string
	var split, timetable, events, officials, coachLevels string
	var ordered bool
	if err := row.Scan(&c.ID, &c.ClubLink, &c.IndividualLink, &c.Name, &c.Date, &deadline, &c.Individuals, &levels, &created, &video, &c.Signoff, &split, &timetable, &events, &officials, &ordered, &c.ApproveCoaches, &coachLevels); err != nil {
		return Competition{}, notFound(err)
	}
	if err := json.Unmarshal([]byte(coachLevels), &c.CoachLevels); err != nil {
		return Competition{}, fmt.Errorf("reading competition %s's coach levels: %w", c.ID, err)
	}
	if err := json.Unmarshal([]byte(officials), &c.Officials); err != nil {
		return Competition{}, fmt.Errorf("reading competition %s's officials: %w", c.ID, err)
	}
	var ev otherEvents
	if err := json.Unmarshal([]byte(events), &ev); err != nil {
		return Competition{}, fmt.Errorf("reading competition %s's events: %w", c.ID, err)
	}
	c.Synchro, c.Tumbling, c.DMT = ev.Synchro, ev.Tumbling, ev.DMT
	if err := json.Unmarshal([]byte(split), &c.Split); err != nil {
		return Competition{}, fmt.Errorf("reading competition %s's split: %w", c.ID, err)
	}
	if timetable != "" {
		c.Timetable = &competitions.Schedule{}
		if err := json.Unmarshal([]byte(timetable), c.Timetable); err != nil {
			return Competition{}, fmt.Errorf("reading competition %s's timetable: %w", c.ID, err)
		}
	}
	if err := json.Unmarshal([]byte(video), &c.Video); err != nil {
		return Competition{}, fmt.Errorf("reading competition %s's video: %w", c.ID, err)
	}
	if err := json.Unmarshal([]byte(levels), &c.Levels); err != nil {
		return Competition{}, fmt.Errorf("reading competition %s's levels: %w", c.ID, err)
	}
	if !ordered {
		c.Levels, c.Synchro = competitions.OrderLevels(nil, c.Levels), competitions.OrderLevels(nil, c.Synchro)
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
	SignedAt  time.Time // when a coach signed it off, as sent (or, for an individual, since); zero if not
	SignedBy  string    // the coach who signed it off, or said not yet
	SignNote  string    // the coach's note
	// SignedCoach is which of the club's coaches signed it off ("" for an
	// individual's coach, or a sign-off from before ADR 0007).
	SignedCoach string
	// Unapproved is a sign-off by a coach the competition hasn't approved, at
	// one that approves coaches (ADR 0007 Decision 7): it doesn't count.
	Unapproved bool
	// Removal is the organiser's: "", Removed or Held, with a note for the
	// club, the member or both (RemovalTo); Resent is a held entry sent again,
	// waiting for the organiser to accept it.
	Removal     string
	RemovalNote string
	RemovalTo   string
	Resent      bool
	// SignoffLink is an individual's link for their coach to sign off the
	// entry (ADR 0004 Decision 11); "" for a club's entry.
	SignoffLink string
	// Coach is the coach the member has chosen or been assigned, now; "" for
	// none, or an individual.
	Coach string
	// PartnerLink is an individual's synchro entry's link for their partner
	// to confirm; PartnerConfirmed says they have (for a club's entry, as sent).
	PartnerLink      string
	PartnerConfirmed bool
	PartnerMemberID  string // the confirmed partner, as a club's member
	PartnerEntryID   string // or as an individual, by their entry
}

// People are the entry's people, by a key that's the same wherever the same
// person appears (ADR 0005 Decision 4): a member by their member id, an
// individual by their entry, and a confirmed synchro partner as whichever
// they confirmed as. An unconfirmed partner is their entry and name.
func (e Entry) People() []string {
	var out []string
	switch {
	case e.MemberID != "":
		out = append(out, "m:"+e.MemberID)
	default:
		out = append(out, "e:"+e.ID)
	}
	if e.Entry.Partner != nil {
		switch {
		case e.PartnerMemberID != "":
			out = append(out, "m:"+e.PartnerMemberID)
		case e.PartnerEntryID != "":
			out = append(out, "e:"+e.PartnerEntryID)
		default:
			out = append(out, "p:"+e.ID)
		}
	}
	return out
}

// CoachName is who coaches the gymnast, for their card: the coach who signed
// the entry off, or else the member's coach.
func (e Entry) CoachName() string {
	if e.SignedOff() {
		return e.SignedBy
	}
	return e.Coach
}

// SignedOff says whether a coach has signed off the entry, one the
// competition approved where it approves coaches.
func (e Entry) SignedOff() bool { return !e.SignedAt.IsZero() && !e.Unapproved }

// signoffCounts is the condition, on a row of entries, for its sign-off to
// count (ADR 0007 Decisions 7 and 8): the competition doesn't approve
// coaches, or the coach who signed is approved there (and signed since, if
// the organiser started their sign-offs afresh): a club's coach, or the
// coach an individual named.
const signoffCounts = `(NOT (SELECT approve_coaches FROM competitions WHERE id = entries.competition_id)
	OR EXISTS (SELECT 1 FROM competition_coaches cc WHERE NOT entries.individual AND cc.competition_id = entries.competition_id
		AND cc.coach_id = entries.signed_coach AND cc.status = 'approved' AND entries.signed_at >= cc.counts_from)
	OR EXISTS (SELECT 1 FROM entry_coaches ec WHERE entries.individual AND ec.entry_id = entries.id
		AND ec.status = 'approved' AND entries.signed_at >= ec.counts_from))`

const (
	// VideoOK and VideoMore are the organiser's review of an entry's videos:
	// they show what's needed, or more is needed (VideoNote says what).
	VideoOK   = "ok"
	VideoMore = "more"
)

// Checked says whether the organiser has checked the entry as it is now.
func (e Entry) Checked() bool { return !e.CheckedAt.IsZero() }

// Entries are everything entered for a competition, but those the organiser
// has removed or holds: clubs' entries by club, then individuals', each by
// gymnast.
func (s *Store) Entries(ctx context.Context, competitionID string) ([]Entry, error) {
	return s.entriesWhere(ctx, `competition_id = $1 AND removal = ''`, competitionID)
}

// entriesWhere are the entries meeting a condition, by club then gymnast.
func (s *Store) entriesWhere(ctx context.Context, where string, args ...any) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, competition_id, COALESCE(club_id, ''), club_name, COALESCE(member_id, ''), individual, entry, sent_at, COALESCE(checked_at, ''), note, video_review, video_note,
		club_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM member_entries me
			WHERE me.member_id = entries.member_id AND me.competition_id = entries.competition_id AND me.discipline = entries.discipline), COALESCE(signed_at, ''), signed_by, sign_note, signoff_token,
		COALESCE((SELECT c.name FROM members m JOIN coaches c ON c.id = m.coach_id WHERE m.id = entries.member_id), ''), partner_token, partner_confirmed, COALESCE(partner_member, ''), COALESCE(partner_entry, ''), signed_coach, `+signoffCounts+`, removal, removal_note, removal_to, resent
		FROM entries WHERE `+where+`
		ORDER BY individual, club_name, gymnast, id`, args...)
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
	var entry, sent, checked, signed string
	var counts bool
	if err := row.Scan(&e.ID, &e.CompetitionID, &e.ClubID, &e.ClubName, &e.MemberID, &e.Individual, &entry, &sent, &checked, &e.Note, &e.VideoReview, &e.VideoNote, &e.Withdrawn, &signed, &e.SignedBy, &e.SignNote, &e.SignoffLink, &e.Coach, &e.PartnerLink, &e.PartnerConfirmed, &e.PartnerMemberID, &e.PartnerEntryID, &e.SignedCoach, &counts, &e.Removal, &e.RemovalNote, &e.RemovalTo, &e.Resent); err != nil {
		return Entry{}, notFound(err)
	}
	e.Unapproved = signed != "" && !counts
	if err := json.Unmarshal([]byte(entry), &e.Entry); err != nil {
		return Entry{}, fmt.Errorf("reading entry %s: %w", e.ID, err)
	}
	e.SentAt, e.CheckedAt, e.SignedAt = parseTime(sent), parseTime(checked), parseTime(signed)
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
	out := Entry{ID: newID(), CompetitionID: competitionID, Individual: true, Entry: e, SentAt: parseTime(s.stamp()), SignoffLink: newToken()}
	if e.Discipline == competitions.Synchro {
		out.PartnerLink = newToken()
	}
	err = s.tx(ctx, func(tx *sql.Tx) error {
		if err := s.open(ctx, tx, competitionID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO entries (id, competition_id, club_name, individual, gymnast, token_hash, entry, sent_at, signoff_token, signoff_hash,
			discipline, partner_token, partner_hash)
			VALUES ($1, $2, '', TRUE, $3, $4, $5, $6, $7, $8, $9, $10, $11)`, out.ID, competitionID, e.Gymnast, hash(token), string(data), out.SentAt.Format(timeLayout),
			out.SignoffLink, hash(out.SignoffLink), e.Discipline, out.PartnerLink, nullHash(out.PartnerLink)); err != nil {
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
	return scanEntry(s.db.QueryRowContext(ctx, `SELECT id, competition_id, '', club_name, '', individual, entry, sent_at, COALESCE(checked_at, ''), note, video_review, video_note, FALSE, COALESCE(signed_at, ''), signed_by, sign_note, signoff_token, '', partner_token, partner_confirmed, COALESCE(partner_member, ''), COALESCE(partner_entry, ''), signed_coach, `+signoffCounts+`, removal, removal_note, removal_to, resent
		FROM entries WHERE token_hash = $1`, hash(token)))
}

// ReplaceIndividualEntry replaces an individual's entry, until the deadline.
func (s *Store) ReplaceIndividualEntry(ctx context.Context, token string, e competitions.Entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		var competitionID, removal string
		if err := tx.QueryRowContext(ctx, `SELECT competition_id, removal FROM entries WHERE token_hash = $1`, hash(token)).Scan(&competitionID, &removal); err != nil {
			return notFound(err)
		}
		if err := s.open(ctx, tx, competitionID); err != nil {
			return err
		}
		if removal == Removed {
			return ErrRemoved
		}
		partner, err := partnerToken(ctx, tx, `SELECT entry, partner_token FROM entries WHERE token_hash = $1`, e, hash(token))
		if err != nil {
			return err
		}
		return affected(tx.ExecContext(ctx, `UPDATE entries SET entry = $1, gymnast = $2, sent_at = $3, discipline = $5,
			resent = resent OR (removal = 'held' AND entry <> $1),
			checked_at = CASE WHEN entry = $1 THEN checked_at ELSE NULL END,
			video_review = CASE WHEN entry = $1 THEN video_review ELSE '' END,
			signed_at = CASE WHEN entry = $1 THEN signed_at ELSE NULL END,
			partner_confirmed = CASE WHEN partner_token = $6 THEN partner_confirmed ELSE FALSE END,
			partner_member = CASE WHEN partner_token = $6 THEN partner_member ELSE NULL END,
			partner_entry = CASE WHEN partner_token = $6 THEN partner_entry ELSE NULL END,
			partner_token = $6, partner_hash = $7 WHERE token_hash = $4`,
			string(data), e.Gymnast, s.stamp(), hash(token), e.Discipline, partner, nullHash(partner)))
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

// SetApproveCoaches says whether only coaches the organiser approves can sign
// off, and the lowest qualification level accepted in each discipline (ADR
// 0007 Decision 2).
func (s *Store) SetApproveCoaches(ctx context.Context, id string, on bool, levels map[string]int) error {
	b, err := json.Marshal(levels)
	if err != nil {
		return err
	}
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET approve_coaches = $1, coach_levels = $2 WHERE id = $3`, on, string(b), id))
}

// SetIndividuals turns a competition's individual entry on or off.
func (s *Store) SetIndividuals(ctx context.Context, id string, on bool) error {
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET individuals = $1 WHERE id = $2`, on, id))
}

// CompetitionEntry is one of a competition's entries, by id.
func (s *Store) CompetitionEntry(ctx context.Context, competitionID, id string) (Entry, error) {
	return scanEntry(s.db.QueryRowContext(ctx, `SELECT id, competition_id, COALESCE(club_id, ''), club_name, COALESCE(member_id, ''), individual, entry, sent_at, COALESCE(checked_at, ''), note, video_review, video_note,
		club_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM member_entries me
			WHERE me.member_id = entries.member_id AND me.competition_id = entries.competition_id AND me.discipline = entries.discipline), COALESCE(signed_at, ''), signed_by, sign_note, signoff_token,
		COALESCE((SELECT c.name FROM members m JOIN coaches c ON c.id = m.coach_id WHERE m.id = entries.member_id), ''), partner_token, partner_confirmed, COALESCE(partner_member, ''), COALESCE(partner_entry, ''), signed_coach, `+signoffCounts+`, removal, removal_note, removal_to, resent
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

// SetSignoff turns on or off whether a competition's entries need a coach's sign-off.
func (s *Store) SetSignoff(ctx context.Context, id string, on bool) error {
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET signoff = $1 WHERE id = $2`, on, id))
}

// EntryBySignoffLink is the individual's entry a coach's sign-off link opens.
func (s *Store) EntryBySignoffLink(ctx context.Context, token string) (Entry, error) {
	return scanEntry(s.db.QueryRowContext(ctx, `SELECT id, competition_id, '', club_name, '', individual, entry, sent_at, COALESCE(checked_at, ''), note, video_review, video_note, FALSE,
		COALESCE(signed_at, ''), signed_by, sign_note, signoff_token, '', partner_token, partner_confirmed, COALESCE(partner_member, ''), COALESCE(partner_entry, ''), signed_coach, `+signoffCounts+`, removal, removal_note, removal_to, resent
		FROM entries WHERE signoff_hash = $1`, hash(token)))
}

// SignOffIndividual records a coach's sign-off of an individual's entry
// (signed false: not yet), under the coach's name, with a note. Changing the
// entry afterwards clears it.
func (s *Store) SignOffIndividual(ctx context.Context, token, coach string, signed bool, note string) error {
	var at any
	if signed {
		at = s.stamp()
	}
	return affected(s.db.ExecContext(ctx, `UPDATE entries SET signed_at = $1, signed_by = $2, sign_note = $3 WHERE signoff_hash = $4`,
		at, coach, note, hash(token)))
}

// SetSplit changes which levels split men and women.
func (s *Store) SetSplit(ctx context.Context, id string, split competitions.Split) error {
	data, err := json.Marshal(split)
	if err != nil {
		return err
	}
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET split = $1 WHERE id = $2`, string(data), id))
}

// SetTimetable saves a competition's timetable and its setup (nil to remove it).
func (s *Store) SetTimetable(ctx context.Context, id string, t *competitions.Schedule) error {
	data := ""
	if t != nil {
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		data = string(b)
	}
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET timetable = $1 WHERE id = $2`, data, id))
}

// SetLevelOrder stores the order of a competition's levels, every discipline's,
// as the organiser left it.
func (s *Store) SetLevelOrder(ctx context.Context, id string, c competitions.Competition) error {
	levels, err := json.Marshal(c.Levels)
	if err != nil {
		return err
	}
	events, err := json.Marshal(otherEvents{c.Synchro, c.Tumbling, c.DMT})
	if err != nil {
		return err
	}
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET levels = $1, events = $2, levels_ordered = TRUE WHERE id = $3`,
		string(levels), string(events), id))
}

// Scenarios are a competition's simulated timetables, oldest first.
func (s *Store) Scenarios(ctx context.Context, id string) ([]competitions.Scenario, error) {
	var data string
	if err := s.db.QueryRowContext(ctx, `SELECT scenarios FROM competitions WHERE id = $1`, id).Scan(&data); err != nil {
		return nil, notFound(err)
	}
	var out []competitions.Scenario
	if err := json.Unmarshal([]byte(data), &out); err != nil {
		return nil, fmt.Errorf("reading competition %s's scenarios: %w", id, err)
	}
	return out, nil
}

// SetScenarios replaces a competition's simulated timetables.
func (s *Store) SetScenarios(ctx context.Context, id string, scenarios []competitions.Scenario) error {
	if scenarios == nil {
		scenarios = []competitions.Scenario{}
	}
	data, err := json.Marshal(scenarios)
	if err != nil {
		return err
	}
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET scenarios = $1 WHERE id = $2`, string(data), id))
}

// otherEvents are a competition's events beyond individual trampoline, as stored.
type otherEvents struct {
	Synchro  []competitions.Level `json:"synchro,omitempty"`
	Tumbling []string             `json:"tumbling,omitempty"`
	DMT      []string             `json:"dmt,omitempty"`
}

// SetEvents changes a competition's synchro, tumbling and DMT levels.
func (s *Store) SetEvents(ctx context.Context, id string, synchro []competitions.Level, tumbling, dmt []string) error {
	data, err := json.Marshal(otherEvents{synchro, tumbling, dmt})
	if err != nil {
		return err
	}
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET events = $1 WHERE id = $2`, string(data), id))
}
