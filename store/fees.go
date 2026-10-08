package store

import (
	"context"
	"encoding/json"
	"time"

	"tariffCalculator/competitions"
)

// Entry fees (roadmap 2026-10-08): the competition's fees, and payments the
// organiser records against a payer: a club ("club:" and its id) or an
// individual's entry ("entry:" and its id).

// SetFees changes what a competition charges.
func (s *Store) SetFees(ctx context.Context, competitionID string, f competitions.Fees) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET fees = $1 WHERE id = $2`, string(data), competitionID))
}

// Payment is money the organiser says a payer has paid.
type Payment struct {
	ID, Payer string
	Amount    int // in cents
	Note, Who string
	At        time.Time
}

// MaxPayments is how many payments a competition can record.
const MaxPayments = 2000

// AddPayment records a payment.
func (s *Store) AddPayment(ctx context.Context, competitionID, payer string, amount int, note, who string) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM payments WHERE competition_id = $1`, competitionID).Scan(&n); err != nil {
		return err
	}
	if n >= MaxPayments {
		return ErrLimit
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO payments (id, competition_id, payer, amount, note, at, who) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		newID(), competitionID, payer, amount, note, s.stamp(), who)
	return err
}

// Payments are a competition's payments, oldest first.
func (s *Store) Payments(ctx context.Context, competitionID string) ([]Payment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, payer, amount, note, who, at FROM payments WHERE competition_id = $1 ORDER BY at, rowid`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Payment
	for rows.Next() {
		var p Payment
		var at string
		if err := rows.Scan(&p.ID, &p.Payer, &p.Amount, &p.Note, &p.Who, &at); err != nil {
			return nil, err
		}
		p.At = parseTime(at)
		out = append(out, p)
	}
	return out, rows.Err()
}

// RemovePayment takes back a payment recorded by mistake.
func (s *Store) RemovePayment(ctx context.Context, competitionID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM payments WHERE competition_id = $1 AND id = $2`, competitionID, id))
}
