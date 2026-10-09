package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"tariffCalculator/competitions"
)

// Officials (ADR 0005 Decisions 5 and 13). A club's members say what they can
// judge and help with; the club sends that with its entries. Individuals offer
// on their own entry, and the organiser adds people with no club. The
// competition's officials list holds them all.

// Official is one of a competition's officials.
type Official struct {
	ID        string
	Name      string
	ClubName  string // "" for none
	MemberID  string // a club's member's offer, as the club sent it
	EntryID   string // an individual's offer, on their entry
	Added     bool   // added by the organiser
	Offer     competitions.Offer
	Qualified bool // the organiser marked them qualified to judge
	UpdatedAt time.Time
}

// SaveMemberOffer keeps what a member offers to do at a competition their club
// is entered in, until the deadline. An empty offer removes it.
func (s *Store) SaveMemberOffer(ctx context.Context, memberID, competitionID string, o competitions.Offer) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var clubID string
		err := tx.QueryRowContext(ctx, `SELECT m.club_id FROM members m
			JOIN club_competitions cc ON cc.club_id = m.club_id AND cc.competition_id = $2
			WHERE m.id = $1`, memberID, competitionID).Scan(&clubID)
		if err == sql.ErrNoRows {
			return ErrNotAttached
		} else if err != nil {
			return err
		}
		if err := s.open(ctx, tx, competitionID, changing); err != nil {
			return err
		}
		if o.Empty() {
			_, err := tx.ExecContext(ctx, `DELETE FROM member_offers WHERE member_id = $1 AND competition_id = $2`, memberID, competitionID)
			return err
		}
		data, err := json.Marshal(o)
		if err != nil {
			return err
		}
		now := s.stamp()
		if _, err := tx.ExecContext(ctx, `INSERT INTO member_offers (member_id, competition_id, offer, updated_at) VALUES ($1, $2, $3, $4)
			ON CONFLICT (member_id, competition_id) DO UPDATE SET offer = excluded.offer, updated_at = excluded.updated_at`,
			memberID, competitionID, string(data), now); err != nil {
			return err
		}
		return touch(ctx, tx, clubID, now)
	})
}

// MemberOffer is what a member offers at a competition (empty for nothing).
func (s *Store) MemberOffer(ctx context.Context, memberID, competitionID string) (competitions.Offer, error) {
	var o competitions.Offer
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT offer FROM member_offers WHERE member_id = $1 AND competition_id = $2`, memberID, competitionID).Scan(&data)
	if err == sql.ErrNoRows {
		return o, nil
	} else if err != nil {
		return o, err
	}
	return o, json.Unmarshal([]byte(data), &o)
}

// ClubOffers are what a club's members offer at a competition, by member.
func (s *Store) ClubOffers(ctx context.Context, clubID, competitionID string) (map[string]competitions.Offer, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT mo.member_id, mo.offer FROM member_offers mo JOIN members m ON m.id = mo.member_id
		WHERE m.club_id = $1 AND mo.competition_id = $2`, clubID, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]competitions.Offer{}
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		var o competitions.Offer
		if err := json.Unmarshal([]byte(data), &o); err != nil {
			return nil, err
		}
		out[id] = o
	}
	return out, rows.Err()
}

// sendOffers makes the competition's officials from a club exactly what its
// members offer now, keeping the organiser's qualified marks. It's part of
// sending the club's entries.
func sendOffers(ctx context.Context, tx *sql.Tx, clubID, clubName, competitionID, now string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM officials WHERE competition_id = $1 AND club_id = $2 AND NOT added
		AND NOT EXISTS (SELECT 1 FROM member_offers mo WHERE mo.competition_id = $1 AND mo.member_id = officials.member_id)`,
		competitionID, clubID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO officials (id, competition_id, club_id, member_id, club_name, name, offer, updated_at)
		SELECT 'o' || $1 || mo.member_id, $1, $2, mo.member_id, $4, m.name, mo.offer, $3
		FROM member_offers mo JOIN members m ON m.id = mo.member_id
		WHERE m.club_id = $2 AND mo.competition_id = $1
		ON CONFLICT (competition_id, member_id) DO UPDATE SET offer = excluded.offer, name = excluded.name,
			club_name = excluded.club_name, updated_at = excluded.updated_at`,
		competitionID, clubID, now, clubName)
	return err
}

// SetIndividualOffer keeps what an individual offers, on their entry, until
// the deadline. An empty offer removes it.
func (s *Store) SetIndividualOffer(ctx context.Context, token string, o competitions.Offer) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var id, competitionID, name string
		if err := tx.QueryRowContext(ctx, `SELECT id, competition_id, gymnast FROM entries WHERE token_hash = $1`, hash(token)).
			Scan(&id, &competitionID, &name); err != nil {
			return notFound(err)
		}
		if err := s.open(ctx, tx, competitionID, changing); err != nil {
			return err
		}
		if o.Empty() {
			_, err := tx.ExecContext(ctx, `DELETE FROM officials WHERE entry_id = $1`, id)
			return err
		}
		data, err := json.Marshal(o)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO officials (id, competition_id, entry_id, name, offer, updated_at) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (competition_id, entry_id) DO UPDATE SET offer = excluded.offer, name = excluded.name, updated_at = excluded.updated_at`,
			newID(), competitionID, id, name, string(data), s.stamp())
		return err
	})
}

// IndividualOffer is what an individual offers, on their entry.
func (s *Store) IndividualOffer(ctx context.Context, entryID string) (competitions.Offer, error) {
	var o competitions.Offer
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT offer FROM officials WHERE entry_id = $1`, entryID).Scan(&data)
	if err == sql.ErrNoRows {
		return o, nil
	} else if err != nil {
		return o, err
	}
	return o, json.Unmarshal([]byte(data), &o)
}

// Officials are a competition's officials, by name.
func (s *Store) Officials(ctx context.Context, competitionID string) ([]Official, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, club_name, COALESCE(member_id, ''), COALESCE(entry_id, ''), added, offer, qualified, updated_at
		FROM officials WHERE competition_id = $1 ORDER BY name, club_name, id`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Official
	for rows.Next() {
		var o Official
		var offer, updated string
		if err := rows.Scan(&o.ID, &o.Name, &o.ClubName, &o.MemberID, &o.EntryID, &o.Added, &offer, &o.Qualified, &updated); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(offer), &o.Offer); err != nil {
			return nil, err
		}
		o.UpdatedAt = parseTime(updated)
		out = append(out, o)
	}
	return out, rows.Err()
}

// AddOfficial adds a person the organiser knows, perhaps with no club.
func (s *Store) AddOfficial(ctx context.Context, competitionID, name, club string, o competitions.Offer) (Official, error) {
	data, err := json.Marshal(o)
	if err != nil {
		return Official{}, err
	}
	out := Official{ID: newID(), Name: name, ClubName: club, Added: true, Offer: o, UpdatedAt: parseTime(s.stamp())}
	_, err = s.db.ExecContext(ctx, `INSERT INTO officials (id, competition_id, club_name, name, offer, added, updated_at) VALUES ($1, $2, $3, $4, $5, TRUE, $6)`,
		out.ID, competitionID, club, name, string(data), out.UpdatedAt.Format(timeLayout))
	return out, err
}

// UpdateOfficial changes what a person the organiser added can do.
func (s *Store) UpdateOfficial(ctx context.Context, competitionID, id string, o competitions.Offer) error {
	data, err := json.Marshal(o)
	if err != nil {
		return err
	}
	return affected(s.db.ExecContext(ctx, `UPDATE officials SET offer = $1, updated_at = $2 WHERE competition_id = $3 AND id = $4 AND added`,
		string(data), s.stamp(), competitionID, id))
}

// SetQualified marks an official qualified to judge, or not.
func (s *Store) SetQualified(ctx context.Context, competitionID, id string, qualified bool) error {
	return affected(s.db.ExecContext(ctx, `UPDATE officials SET qualified = $1 WHERE competition_id = $2 AND id = $3`, qualified, competitionID, id))
}

// RemoveOfficial removes a person the organiser added. People who offered
// themselves stay while they offer.
func (s *Store) RemoveOfficial(ctx context.Context, competitionID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM officials WHERE competition_id = $1 AND id = $2 AND added`, competitionID, id))
}

// SetOfficialSettings changes a competition's panels and judging rule.
func (s *Store) SetOfficialSettings(ctx context.Context, id string, settings competitions.OfficialSettings) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET officials = $1 WHERE id = $2`, string(data), id))
}
