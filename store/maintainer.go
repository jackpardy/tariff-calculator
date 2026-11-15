package store

import (
	"context"
	"strings"
	"time"
)

// Found is a competition or a club the maintainer's find command lists:
// nothing secret, enough to tell it from another of the same name.
type Found struct {
	Kind  string // "competition" or "club"
	ID    string
	Name  string
	Date  string    // a competition's day, as DateLayout; "" for a club
	Used  time.Time // a club's last use; zero for a competition
	Count int       // a competition's entries, or a club's members
}

// Find lists the competitions and clubs whose name contains every one of
// the words, ignoring case and accents, competitions first.
func (s *Store) Find(ctx context.Context, words string) ([]Found, error) {
	want := strings.Fields(fold(words))
	match := func(name string) bool {
		name = fold(name)
		for _, w := range want {
			if !strings.Contains(name, w) {
				return false
			}
		}
		return true
	}
	var out []Found
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.name, c.date, (SELECT COUNT(*) FROM entries e WHERE e.competition_id = c.id)
		FROM competitions c ORDER BY c.date, c.name`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		f := Found{Kind: "competition"}
		if err := rows.Scan(&f.ID, &f.Name, &f.Date, &f.Count); err != nil {
			rows.Close()
			return nil, err
		}
		if match(f.Name) {
			out = append(out, f)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT c.id, c.name, c.last_used, (SELECT COUNT(*) FROM members m WHERE m.club_id = c.id)
		FROM clubs c ORDER BY c.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		f := Found{Kind: "club"}
		var used string
		if err := rows.Scan(&f.ID, &f.Name, &used, &f.Count); err != nil {
			return nil, err
		}
		f.Used = parseTime(used)
		if match(f.Name) {
			out = append(out, f)
		}
	}
	return out, rows.Err()
}

// accents folds the accented letters names are written with to plain ones.
var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ø", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ý", "y", "ÿ", "y", "ñ", "n", "ç", "c", "ß", "ss", "æ", "ae",
)

// fold is s in lower case without accents.
func fold(s string) string { return accents.Replace(strings.ToLower(s)) }
