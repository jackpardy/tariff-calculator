package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"slices"
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
	EntryKey  string // that entry's person key ("" if it has none)
	Added     bool   // added by the organiser
	Offer     competitions.Offer
	Qualified bool // the organiser marked them qualified to judge
	UpdatedAt time.Time
	// TakenBackAt is when their club took their offer back (taking them out
	// of the competition, or removing them), by whom (TakenBackBy, "" for
	// the club's record expiring), and PersonKey the member's key they had:
	// only TakenBack reads these.
	TakenBackAt time.Time
	TakenBackBy string
	PersonKey   string
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

// HeldOffer is what the organiser holds of a club member's offer: the copy
// made when the club last sent its entries.
type HeldOffer struct {
	MemberID string
	Name     string
	Offer    competitions.Offer
}

// ClubHeldOffers are the offers the organiser holds from a club's members at
// a competition, by member: what the club has sent, to compare with what
// members offer now (ClubOffers). The judging qualifications and certificates
// beside an offer aren't copied: the organiser reads them from the member's.
func (s *Store) ClubHeldOffers(ctx context.Context, clubID, competitionID string) (map[string]HeldOffer, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT member_id, name, offer FROM officials
		WHERE competition_id = $1 AND club_id = $2 AND NOT added AND COALESCE(member_id, '') <> '' AND taken_back_at = ''`, competitionID, clubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]HeldOffer{}
	for rows.Next() {
		var h HeldOffer
		var data string
		if err := rows.Scan(&h.MemberID, &h.Name, &data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &h.Offer); err != nil {
			return nil, err
		}
		out[h.MemberID] = h
	}
	return out, rows.Err()
}

// sendOffers makes the competition's officials from a club exactly what its
// members offer now, keeping the organiser's qualified marks. It's part of
// sending the club's entries.
func sendOffers(ctx context.Context, tx *sql.Tx, clubID, clubName, competitionID, now string) error {
	// An offer the club took back stays, marked, until the member offers
	// again (keepOffers).
	if _, err := tx.ExecContext(ctx, `DELETE FROM officials WHERE competition_id = $1 AND club_id = $2 AND NOT added AND taken_back_at = ''
		AND NOT EXISTS (SELECT 1 FROM member_offers mo WHERE mo.competition_id = $1 AND mo.member_id = officials.member_id)`,
		competitionID, clubID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO officials (id, competition_id, club_id, member_id, club_name, name, offer, updated_at)
		SELECT 'o' || $1 || mo.member_id, $1, $2, mo.member_id, $4, m.name, mo.offer, $3
		FROM member_offers mo JOIN members m ON m.id = mo.member_id
		WHERE m.club_id = $2 AND mo.competition_id = $1
		ON CONFLICT (competition_id, member_id) DO UPDATE SET offer = excluded.offer, name = excluded.name,
			club_name = excluded.club_name, updated_at = excluded.updated_at, taken_back_at = '', taken_back_by = '', person_key = ''`,
		competitionID, clubID, now, clubName)
	return err
}

// sameGymnast are an individual's entries at the competition of entry $1
// for the same gymnast, that entry among them: their offer is one for the
// competition, kept on one of them.
const sameGymnast = `SELECT $1 UNION SELECT e.id FROM entries e JOIN individuals i ON i.id = e.individual_id
	JOIN entries x ON x.id = $1 JOIN individuals xi ON xi.id = x.individual_id
	WHERE e.individual AND e.competition_id = x.competition_id AND i.person_id = xi.person_id AND lower(e.gymnast) = lower(x.gymnast)`

// SetIndividualOffer keeps what an individual offers at the competition of
// their entry (one offer whichever of their entries it's made on), until the
// deadline. An empty offer removes it.
func (s *Store) SetIndividualOffer(ctx context.Context, token string, o competitions.Offer) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		id, err := individualEntryID(ctx, tx, token)
		if err != nil {
			return err
		}
		var competitionID, name string
		if err := tx.QueryRowContext(ctx, `SELECT competition_id, gymnast FROM entries WHERE id = $1`, id).
			Scan(&competitionID, &name); err != nil {
			return notFound(err)
		}
		if err := s.open(ctx, tx, competitionID, changing); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM officials WHERE entry_id IN (`+sameGymnast+`)`, id); err != nil || o.Empty() {
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

// IndividualOffer is what an individual offers at the competition of their
// entry, made on it or another of theirs there.
func (s *Store) IndividualOffer(ctx context.Context, entryID string) (competitions.Offer, error) {
	var o competitions.Offer
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT offer FROM officials WHERE entry_id IN (`+sameGymnast+`) ORDER BY updated_at DESC LIMIT 1`, entryID).Scan(&data)
	if err == sql.ErrNoRows {
		return o, nil
	} else if err != nil {
		return o, err
	}
	return o, json.Unmarshal([]byte(data), &o)
}

// Officials are a competition's officials, by name. An individual whose
// entries there the organiser has all removed isn't one, until one is put
// back: their offer goes with them, as it does when they withdraw. One held
// for changes keeps it. One whose club took their offer back isn't one
// either (TakenBack lists them).
func (s *Store) Officials(ctx context.Context, competitionID string) ([]Official, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, club_name, COALESCE(member_id, ''), COALESCE(entry_id, ''), added, offer, qualified, updated_at,
			COALESCE((SELECT person_key FROM entries WHERE id = officials.entry_id), '')
		FROM officials WHERE competition_id = $1 AND taken_back_at = ''
		AND NOT (COALESCE((SELECT removal FROM entries WHERE id = officials.entry_id), '') = $2
			AND NOT EXISTS (SELECT 1 FROM entries x JOIN individuals xi ON xi.id = x.individual_id
				JOIN entries e ON e.competition_id = x.competition_id AND e.individual AND lower(e.gymnast) = lower(x.gymnast)
				JOIN individuals i ON i.id = e.individual_id AND i.person_id = xi.person_id
				WHERE x.id = officials.entry_id AND e.removal NOT IN ($2, $3)))
		ORDER BY name, club_name, id`, competitionID, Removed, Withdrew)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Official
	for rows.Next() {
		var o Official
		var offer, updated string
		if err := rows.Scan(&o.ID, &o.Name, &o.ClubName, &o.MemberID, &o.EntryID, &o.Added, &offer, &o.Qualified, &updated, &o.EntryKey); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(offer), &o.Offer); err != nil {
			return nil, err
		}
		o.UpdatedAt = parseTime(updated)
		out = append(out, o)
	}
	slices.SortStableFunc(out, func(a, b Official) int {
		return cmp.Or(competitions.CompareNames(a.Name, b.Name), competitions.CompareNames(a.ClubName, b.ClubName))
	})
	return out, rows.Err()
}

// TakenBack are a competition's officials whose club took their offer back,
// by name: listed for the organiser, but in no rota or seat.
func (s *Store) TakenBack(ctx context.Context, competitionID string) ([]Official, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, club_name, COALESCE(member_id, ''), offer, updated_at, taken_back_at, taken_back_by, person_key
		FROM officials WHERE competition_id = $1 AND taken_back_at <> '' ORDER BY name, club_name, id`, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Official
	for rows.Next() {
		var o Official
		var offer, updated, at string
		if err := rows.Scan(&o.ID, &o.Name, &o.ClubName, &o.MemberID, &offer, &updated, &at, &o.TakenBackBy, &o.PersonKey); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(offer), &o.Offer); err != nil {
			return nil, err
		}
		o.UpdatedAt, o.TakenBackAt = parseTime(updated), parseTime(at)
		out = append(out, o)
	}
	slices.SortStableFunc(out, func(a, b Official) int {
		return cmp.Or(competitions.CompareNames(a.Name, b.Name), competitions.CompareNames(a.ClubName, b.ClubName))
	})
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

// carryOffers finds the offers of a competition, kept as a member made them
// (member_offers) and as the competition holds them (officials: sent by a
// club, an individual's own, or added by the organiser), that say "up to" a
// level in a discipline (from), and with write rewrites them to say to. It
// says how many people: a member whose offer was sent has both.
func carryOffers(ctx context.Context, tx *sql.Tx, competitionID, discipline, from, to string, write bool) (int, error) {
	people := map[string]bool{}
	for _, t := range []struct{ query, update string }{
		{`SELECT member_id, member_id, offer FROM member_offers WHERE competition_id = $1`,
			`UPDATE member_offers SET offer = $2 WHERE member_id = $1 AND competition_id = $3`},
		{`SELECT id, COALESCE(NULLIF(member_id, ''), id), offer FROM officials WHERE competition_id = $1`,
			`UPDATE officials SET offer = $2 WHERE id = $1 AND competition_id = $3`},
	} {
		rows, err := tx.QueryContext(ctx, t.query, competitionID)
		if err != nil {
			return 0, err
		}
		changed := map[string]string{}
		for rows.Next() {
			var id, person, data string
			if err := rows.Scan(&id, &person, &data); err != nil {
				rows.Close()
				return 0, err
			}
			var o competitions.Offer
			if err := json.Unmarshal([]byte(data), &o); err != nil {
				rows.Close()
				return 0, err
			}
			if carried, ok := o.Carried(discipline, from, to); ok {
				people[person] = true
				if out, err := json.Marshal(carried); err == nil {
					changed[id] = string(out)
				}
			}
		}
		if err := rows.Close(); err != nil {
			return 0, err
		}
		if write {
			for id, data := range changed {
				if _, err := tx.ExecContext(ctx, t.update, id, data, competitionID); err != nil {
					return 0, err
				}
			}
		}
	}
	return len(people), nil
}

// LevelOffers is how many people's offers at a competition say they'll judge
// up to a level (up to means any role).
func (s *Store) LevelOffers(ctx context.Context, competitionID, discipline, level string) (int, error) {
	var n int
	err := s.tx(ctx, func(tx *sql.Tx) (err error) {
		n, err = carryOffers(ctx, tx, competitionID, discipline, level, level, false)
		return
	})
	return n, err
}

// CarryLevels carries renamed levels over in every offer of the competition,
// in one go, both the copies members made and what the competition holds, so
// a club's next send keeps the new name. It says how many people each
// rename carried, and forgets the renames carried.
func (s *Store) CarryLevels(ctx context.Context, competitionID string, renames []competitions.LevelRename) ([]int, error) {
	counts := make([]int, len(renames))
	err := s.tx(ctx, func(tx *sql.Tx) error {
		for i, r := range renames {
			n, err := carryOffers(ctx, tx, competitionID, r.Discipline, r.From, r.To, true)
			if err != nil {
				return err
			}
			counts[i] = n
		}
		var data string
		if err := tx.QueryRowContext(ctx, `SELECT officials FROM competitions WHERE id = $1`, competitionID).Scan(&data); err != nil {
			return notFound(err)
		}
		var settings competitions.OfficialSettings
		if data != "" {
			if err := json.Unmarshal([]byte(data), &settings); err != nil {
				return err
			}
		}
		settings.Renamed = slices.DeleteFunc(settings.Renamed, func(k competitions.LevelRename) bool { return slices.Contains(renames, k) })
		out, err := json.Marshal(settings)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE competitions SET officials = $1 WHERE id = $2`, string(out), competitionID)
		return err
	})
	return counts, err
}
