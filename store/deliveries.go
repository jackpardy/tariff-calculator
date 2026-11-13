package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Deliveries (migration 58): every email and push the app sends, kept from
// when it's decided until it goes, so neither a restart nor a failed send
// loses one. The sender's workers take them in turn; one sent is deleted,
// as it holds names; one that fails after its last try is kept as failed
// for FailedKeptDays, for the organiser's counts.

// What a delivery is.
const (
	DeliverEmail = "email"
	DeliverPush  = "push"
)

// A delivery's state.
const (
	deliveryWaiting = "waiting"
	deliverySending = "sending"
	deliveryFailed  = "failed"
)

// What a round of deliveries is.
const (
	RoundChanges = "changes" // change notifications (ADR 0008)
	RoundDesk    = "desk"    // a desk message, whose id the round has
)

// FailedKeptDays is how long a delivery that couldn't be sent is kept, to
// say who wasn't told and why.
const FailedKeptDays = 7

// Delivery is one email or push to one address.
type Delivery struct {
	ID             string
	Kind           string // DeliverEmail or DeliverPush
	CompetitionID  string // "" for none
	RoundID        string // the round it's counted in, "" for none
	SubscriptionID string // the subscription it's for, "" for none
	Drop           []string
	// Drop are the push subscriptions dropped if the push service says
	// it's gone.
	Who     string // whose subscription it is, in words (SubscriberName), for the organiser
	About   string // what a failure is logged as, e.g. "Desk: emailing about <competition id>"
	Address string // the email address, or the push endpoint
	Keys    string // a push subscription's keys
	Subject string // an email's
	Body    string // an email's text, or a push's JSON
	Headers map[string]string
	Tries   int    // how many times it's been tried, this one included once taken
	Why     string // why it failed, in plain words
	MadeAt  time.Time
}

// Round is a desk message's deliveries, or one round of change
// notifications', with how they went.
type Round struct {
	ID            string
	CompetitionID string
	Kind          string // RoundChanges or RoundDesk
	At            time.Time
	Pushed        int // sent by push
	Emailed       int // sent by email
	Failed        int // couldn't be sent
	Sending       int // still to go
}

// AddDeliveries keeps deliveries to send, and their round, if it has a kind.
func (s *Store) AddDeliveries(ctx context.Context, round Round, ds []Delivery) error {
	if len(ds) == 0 {
		return nil
	}
	return s.tx(ctx, func(tx *sql.Tx) error { return s.addDeliveries(ctx, tx, round, ds) })
}

// addDeliveries is AddDeliveries in a transaction.
func (s *Store) addDeliveries(ctx context.Context, tx *sql.Tx, round Round, ds []Delivery) error {
	now := s.stamp()
	if round.Kind != "" {
		if round.ID == "" {
			round.ID = newID()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO delivery_rounds (id, competition_id, kind, made_at) VALUES ($1, $2, $3, $4)`,
			round.ID, round.CompetitionID, round.Kind, now); err != nil {
			return err
		}
	} else {
		round.ID = ""
	}
	for _, d := range ds {
		headers, err := json.Marshal(d.Headers)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO deliveries (id, kind, competition_id, round_id, subscription_id, drop_ids, who, about, address, keys, subject, body, headers, made_at, next_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14)`,
			newID(), d.Kind, d.CompetitionID, round.ID, d.SubscriptionID, strings.Join(d.Drop, ","), d.Who, d.About, d.Address, d.Keys, d.Subject, d.Body, string(headers), now); err != nil {
			return err
		}
	}
	return nil
}

const deliveryColumns = `id, kind, competition_id, round_id, subscription_id, drop_ids, who, about, address, keys, subject, body, headers, tries, why, made_at`

func scanDelivery(row interface{ Scan(...any) error }) (Delivery, error) {
	var d Delivery
	var drop, headers, made string
	if err := row.Scan(&d.ID, &d.Kind, &d.CompetitionID, &d.RoundID, &d.SubscriptionID, &drop, &d.Who, &d.About, &d.Address, &d.Keys, &d.Subject, &d.Body, &headers, &d.Tries, &d.Why, &made); err != nil {
		return Delivery{}, err
	}
	if drop != "" {
		d.Drop = strings.Split(drop, ",")
	}
	json.Unmarshal([]byte(headers), &d.Headers)
	d.MadeAt = parseTime(made)
	return d, nil
}

// TakeDelivery takes the next delivery due, marking it being sent and
// counting the try; false when none is due.
func (s *Store) TakeDelivery(ctx context.Context) (Delivery, bool, error) {
	// A read first: a transaction takes the write lock, which a worker
	// finding nothing to send shouldn't.
	var due int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM deliveries WHERE state = $1 AND next_at <= $2`, deliveryWaiting, s.stamp()).Scan(&due); err != nil || due == 0 {
		return Delivery{}, false, err
	}
	var d Delivery
	found := false
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var err error
		d, err = scanDelivery(tx.QueryRowContext(ctx, `SELECT `+deliveryColumns+` FROM deliveries WHERE state = $1 AND next_at <= $2 ORDER BY next_at, made_at, rowid LIMIT 1`,
			deliveryWaiting, s.stamp()))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		d.Tries++
		_, err = tx.ExecContext(ctx, `UPDATE deliveries SET state = $1, tries = $2 WHERE id = $3`, deliverySending, d.Tries, d.ID)
		return err
	})
	return d, found && err == nil, err
}

// NextDelivery is when the next delivery waiting is due; false for none.
func (s *Store) NextDelivery(ctx context.Context) (time.Time, bool, error) {
	var at sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MIN(next_at) FROM deliveries WHERE state = $1`, deliveryWaiting).Scan(&at); err != nil {
		return time.Time{}, false, err
	}
	if !at.Valid {
		return time.Time{}, false, nil
	}
	return parseTime(at.String), true, nil
}

// DeliveriesDue is how many deliveries are due now or being sent.
func (s *Store) DeliveriesDue(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM deliveries WHERE state = $1 OR (state = $2 AND next_at <= $3)`,
		deliverySending, deliveryWaiting, s.stamp()).Scan(&n)
	return n, err
}

// Delivered deletes a delivery that's been sent, counting it on its round,
// unless it's gone already (sent by another run after a restart).
func (s *Store) Delivered(ctx context.Context, d Delivery) error {
	column := "emailed"
	if d.Kind == DeliverPush {
		column = "pushed"
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM deliveries WHERE id = $1`, d.ID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n == 0 || d.RoundID == "" {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE delivery_rounds SET `+column+` = `+column+` + 1 WHERE id = $1`, d.RoundID)
		return err
	})
}

// DeliveryFailed records why a try failed: tried again at retry, or, when
// retry is zero, kept as failed and counted so on its round. Only one
// being sent is changed.
func (s *Store) DeliveryFailed(ctx context.Context, d Delivery, why string, retry time.Time) error {
	if !retry.IsZero() {
		_, err := s.db.ExecContext(ctx, `UPDATE deliveries SET state = $1, next_at = $2, why = $3 WHERE id = $4 AND state = $5`,
			deliveryWaiting, formatTime(retry), why, d.ID, deliverySending)
		return err
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		// Its body and keys go now: only who and why are shown.
		res, err := tx.ExecContext(ctx, `UPDATE deliveries SET state = $1, next_at = $2, why = $3, body = '', keys = '', headers = '{}' WHERE id = $4 AND state = $5`,
			deliveryFailed, s.stamp(), why, d.ID, deliverySending)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n == 0 || d.RoundID == "" {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE delivery_rounds SET failed = failed + 1 WHERE id = $1`, d.RoundID)
		return err
	})
}

// ResumeDeliveries puts back, when the app starts, what was being sent when
// it stopped: sent again, as whether it went isn't known.
func (s *Store) ResumeDeliveries(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE deliveries SET state = $1 WHERE state = $2`, deliveryWaiting, deliverySending)
	return err
}

// LatestRound is a competition's latest round of a kind, with how it went;
// false for none.
func (s *Store) LatestRound(ctx context.Context, competitionID, kind string) (Round, bool, error) {
	var r Round
	var at string
	err := s.db.QueryRowContext(ctx, `SELECT id, competition_id, kind, made_at, pushed, emailed, failed,
			(SELECT COUNT(*) FROM deliveries WHERE round_id = delivery_rounds.id AND state <> $3)
		FROM delivery_rounds WHERE competition_id = $1 AND kind = $2 ORDER BY made_at DESC, rowid DESC LIMIT 1`, competitionID, kind, deliveryFailed).
		Scan(&r.ID, &r.CompetitionID, &r.Kind, &at, &r.Pushed, &r.Emailed, &r.Failed, &r.Sending)
	if errors.Is(err, sql.ErrNoRows) {
		return Round{}, false, nil
	}
	if err != nil {
		return Round{}, false, err
	}
	r.At = parseTime(at)
	return r, true, nil
}

// FailedDeliveries are a round's deliveries that couldn't be sent.
func (s *Store) FailedDeliveries(ctx context.Context, roundID string) ([]Delivery, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+deliveryColumns+` FROM deliveries WHERE round_id = $1 AND state = $2 ORDER BY who, rowid`, roundID, deliveryFailed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Delivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// dropOldDeliveries deletes failed deliveries kept FailedKeptDays, rounds of
// change notifications as old, and both of competitions that are gone
// (DeleteExpired).
func dropOldDeliveries(ctx context.Context, tx *sql.Tx, now time.Time) error {
	old := now.AddDate(0, 0, -FailedKeptDays).UTC().Format(timeLayout)
	if _, err := tx.ExecContext(ctx, `DELETE FROM deliveries WHERE state = $1 AND next_at <= $2`, deliveryFailed, old); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM delivery_rounds WHERE kind = $1 AND made_at <= $2`, RoundChanges, old); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM deliveries WHERE competition_id <> '' AND competition_id NOT IN (SELECT id FROM competitions)`); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM delivery_rounds WHERE competition_id NOT IN (SELECT id FROM competitions)`)
	return err
}

// SubscriberName is whose a subscription is, in words: "Ann Ryan", "UCD's
// comp sec" or "Mary Byrne (coach)"; never its address.
func (s *Store) SubscriberName(ctx context.Context, sub Subscription) string {
	table, format := "members", "%s"
	switch sub.Kind {
	case NotifyIndividual:
		table = "individuals"
	case NotifyClub:
		table, format = "clubs", "%s's comp sec"
	case NotifyCoach:
		table, format = "coaches", "%s (coach)"
	}
	var name string
	if err := s.db.QueryRowContext(ctx, `SELECT name FROM `+table+` WHERE id = $1`, sub.OwnerID).Scan(&name); err != nil || name == "" {
		return "Someone"
	}
	return fmt.Sprintf(format, name)
}

// DueChanges are the competitions whose waiting changes are due, with
// their baselines. Nothing is cleared until ToldChanges.
func (s *Store) DueChanges(ctx context.Context) ([]Due, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, notify_baseline FROM competitions WHERE notify_due <> '' AND notify_due <= $1`, s.stamp())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Due
	for rows.Next() {
		var d Due
		var baseline string
		if err := rows.Scan(&d.CompetitionID, &baseline); err != nil {
			return nil, err
		}
		d.Baseline = []byte(baseline)
		out = append(out, d)
	}
	return out, rows.Err()
}

// ToldChanges clears a competition's baseline, so changes made from now on
// start a new wait, and keeps the deliveries telling it, with their round,
// in one transaction: a failure keeps neither, and the changes stay due.
// False if the baseline isn't the one told (another run told it first).
func (s *Store) ToldChanges(ctx context.Context, d Due, ds []Delivery) (bool, error) {
	told := false
	err := s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE competitions SET notify_baseline = '', notify_due = '' WHERE id = $1 AND notify_due <> '' AND notify_baseline = $2`,
			d.CompetitionID, string(d.Baseline))
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n == 0 {
			return err
		}
		told = true
		if len(ds) == 0 {
			return nil
		}
		return s.addDeliveries(ctx, tx, Round{CompetitionID: d.CompetitionID, Kind: RoundChanges}, ds)
	})
	return told && err == nil, err
}
