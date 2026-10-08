package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

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

// PairEntry is a synchro entry as its partner sees it, once they've
// confirmed: the pair's entry, which the other gymnast entered and changes
// (roadmap 2026-10-08).
type PairEntry struct {
	CompetitionID   string
	Entry           competitions.Entry
	Entrant         string // the entering gymnast's name
	Club            string // the entering gymnast's club; "" for an individual
	EntrantMemberID string // the entering gymnast, if a member
	PartnerMemberID string // the partner, if a member
	PartnerEntryID  string // or, as an individual, their entry
	CopyID          string // the competition's copy, once it's there; "" before
	SignedAt        time.Time
	SignedBy        string
}

// pairEntries are confirmed synchro entries whose partner meets cond, a
// condition on a row's partner_member or partner_entry written with %[1]s
// for the row: clubs' entries, then individuals'.
func (s *Store) pairEntries(ctx context.Context, cond string, args ...any) ([]PairEntry, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT me.competition_id, me.entry, m.name, c.name, me.member_id, COALESCE(me.partner_member, ''), COALESCE(me.partner_entry, ''),
			COALESCE((SELECT e.id FROM entries e WHERE e.member_id = me.member_id AND e.competition_id = me.competition_id AND e.discipline = me.discipline), ''),
			COALESCE(me.signed_at, ''), me.signed_by
		FROM member_entries me JOIN members m ON m.id = me.member_id JOIN clubs c ON c.id = m.club_id
		WHERE me.partner_confirmed AND %[1]s
		UNION ALL
		SELECT e.competition_id, e.entry, e.gymnast, '', '', COALESCE(e.partner_member, ''), COALESCE(e.partner_entry, ''), e.id, COALESCE(e.signed_at, ''), e.signed_by
		FROM entries e WHERE e.individual AND e.partner_confirmed AND %[2]s`, fmt.Sprintf(cond, "me"), fmt.Sprintf(cond, "e")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PairEntry
	for rows.Next() {
		var p PairEntry
		var entry, signed string
		if err := rows.Scan(&p.CompetitionID, &entry, &p.Entrant, &p.Club, &p.EntrantMemberID, &p.PartnerMemberID, &p.PartnerEntryID, &p.CopyID, &signed, &p.SignedBy); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(entry), &p.Entry); err != nil {
			return nil, err
		}
		p.SignedAt = parseTime(signed)
		out = append(out, p)
	}
	return out, rows.Err()
}

// MemberPairEntries are the synchro entries a member is the confirmed
// partner in.
func (s *Store) MemberPairEntries(ctx context.Context, memberID string) ([]PairEntry, error) {
	return s.pairEntries(ctx, `%s.partner_member = $1`, memberID)
}

// IndividualPairEntries are those an individual (by their entry) is the
// confirmed partner in.
func (s *Store) IndividualPairEntries(ctx context.Context, entryID string) ([]PairEntry, error) {
	return s.pairEntries(ctx, `%s.partner_entry = $1`, entryID)
}

// CoachPairEntries are those whose partner is one of the members a coach
// sees.
func (s *Store) CoachPairEntries(ctx context.Context, c Coach) ([]PairEntry, error) {
	return s.pairEntries(ctx, `%s.partner_member IN (SELECT m.id FROM members m WHERE `+coachSees+`)`, c.ID, c.ClubID)
}

// ClubPairEntries are those whose partner is one of a club's members.
func (s *Store) ClubPairEntries(ctx context.Context, clubID string) ([]PairEntry, error) {
	return s.pairEntries(ctx, `%s.partner_member IN (SELECT id FROM members WHERE club_id = $1)`, clubID)
}
