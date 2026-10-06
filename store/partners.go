package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"tariffCalculator/competitions"
)

// Synchro partners (ADR 0005 Decision 3): a synchro entry names a partner,
// any gymnast, and gets a partner link to send them. The partner confirms
// from their own member or individual page; until then they're a name only.

// partnerToken is the partner link an entry should have: none unless it's
// synchro; the one it has, if its partner is unchanged; else a new one. query
// selects the stored entry's JSON and partner_token.
func partnerToken(ctx context.Context, tx *sql.Tx, query string, e competitions.Entry, args ...any) (string, error) {
	if e.Discipline != competitions.Synchro || e.Partner == nil {
		return "", nil
	}
	var stored, token string
	switch err := tx.QueryRowContext(ctx, query, args...).Scan(&stored, &token); {
	case err == sql.ErrNoRows:
		return newToken(), nil
	case err != nil:
		return "", err
	}
	var before competitions.Entry
	if json.Unmarshal([]byte(stored), &before) == nil && token != "" && before.Partner != nil && *before.Partner == *e.Partner {
		return token, nil
	}
	return newToken(), nil
}

// nullHash is a token's hash, or NULL for none (so unique indexes allow many).
func nullHash(token string) any {
	if token == "" {
		return nil
	}
	return hash(token)
}

// PartnerInvite is a synchro entry, as its partner link shows it.
type PartnerInvite struct {
	CompetitionID string
	Entry         competitions.Entry
	Club          string // the entering gymnast's club; "" for an individual
	Confirmed     bool
}

// PartnerByLink is the synchro entry a partner link belongs to.
func (s *Store) PartnerByLink(ctx context.Context, token string) (PartnerInvite, error) {
	var p PartnerInvite
	var entry string
	err := s.db.QueryRowContext(ctx, `SELECT me.competition_id, me.entry, c.name, me.partner_confirmed FROM member_entries me
		JOIN members m ON m.id = me.member_id JOIN clubs c ON c.id = m.club_id
		WHERE me.partner_hash = $1`, hash(token)).Scan(&p.CompetitionID, &entry, &p.Club, &p.Confirmed)
	if err == sql.ErrNoRows {
		err = s.db.QueryRowContext(ctx, `SELECT competition_id, entry, '', partner_confirmed FROM entries WHERE partner_hash = $1`, hash(token)).
			Scan(&p.CompetitionID, &entry, &p.Club, &p.Confirmed)
	}
	if err != nil {
		return PartnerInvite{}, notFound(err)
	}
	return p, json.Unmarshal([]byte(entry), &p.Entry)
}

// ConfirmPartner records that the partner confirmed the pair, as a club's
// member (memberID), an individual entered in the competition (entryID), or
// neither (both ""), so the timetable can check the partner's clashes.
func (s *Store) ConfirmPartner(ctx context.Context, token, memberID, entryID string) error {
	null := func(v string) any {
		if v == "" {
			return nil
		}
		return v
	}
	res, err := s.db.ExecContext(ctx, `UPDATE member_entries SET partner_confirmed = TRUE, partner_member = $1, partner_entry = $2
		WHERE partner_hash = $3`, null(memberID), null(entryID), hash(token))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	return affected(s.db.ExecContext(ctx, `UPDATE entries SET partner_confirmed = TRUE, partner_member = $1, partner_entry = $2
		WHERE partner_hash = $3`, null(memberID), null(entryID), hash(token)))
}
