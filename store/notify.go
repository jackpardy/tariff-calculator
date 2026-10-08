package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"
)

// Notifications (ADR 0008): who has opted in to hear about a competition,
// and each competition's baseline of what was live before the changes
// waiting to be told, with when they're due.

// Who a subscription is for: the page it was made on.
const (
	NotifyMember     = "member"     // a club member, by member id
	NotifyIndividual = "individual" // an individual's entry, by entry id
	NotifyClub       = "club"       // the comp sec, by club id
	NotifyCoach      = "coach"      // a coach, by coach id
)

// How a subscription is told.
const (
	ByEmail = "email"
	ByPush  = "push"
)

// What a subscription is about.
const (
	AboutTimetable = "timetable" // the flights they're in
	AboutDuties    = "duties"    // their officiating
	AboutCards     = "cards"     // checked, a note, removed or held
)

// Topics are every topic, in order.
var Topics = []string{AboutTimetable, AboutDuties, AboutCards}

// MaxPush is how many phones one person can have told, per competition.
const MaxPush = 5

// Subscription is one way one person is told about one competition.
type Subscription struct {
	ID            string
	CompetitionID string
	Kind          string // NotifyMember, NotifyIndividual, NotifyClub or NotifyCoach
	OwnerID       string
	Channel       string // ByEmail or ByPush
	Address       string // the email, or the push endpoint
	Keys          string // a push subscription's keys, as the browser gave them (JSON)
	Page          string // the path of the person's page for the competition, to link to
	Token         string // for its confirm and off links
	Topics        []string
	Confirmed     bool // an email is confirmed; push needs no confirming
	CreatedAt     time.Time
}

// About says whether it's about a topic.
func (s Subscription) About(topic string) bool { return slices.Contains(s.Topics, topic) }

const subscriptionColumns = `id, competition_id, kind, owner_id, channel, address, keys, page, token, topics, confirmed, created_at`

func scanSubscription(row interface{ Scan(...any) error }) (Subscription, error) {
	var sub Subscription
	var topics, created string
	if err := row.Scan(&sub.ID, &sub.CompetitionID, &sub.Kind, &sub.OwnerID, &sub.Channel, &sub.Address, &sub.Keys, &sub.Page, &sub.Token, &topics, &sub.Confirmed, &created); err != nil {
		return Subscription{}, notFound(err)
	}
	if topics != "" {
		sub.Topics = strings.Split(topics, ",")
	}
	sub.CreatedAt = parseTime(created)
	return sub, nil
}

func (s *Store) subscriptions(ctx context.Context, where string, args ...any) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+subscriptionColumns+` FROM notify_subscriptions WHERE `+where+` ORDER BY created_at, rowid`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// cleanTopics keeps the known topics, in order.
func cleanTopics(topics []string) string {
	var out []string
	for _, t := range Topics {
		if slices.Contains(topics, t) {
			out = append(out, t)
		}
	}
	return strings.Join(out, ",")
}

// Subscribe saves how someone is told about a competition, returning it and
// a token for its confirm and off links. The token is kept as it is, not
// hashed, to put in each email's off link; it can only confirm or turn off
// that one subscription. An email replaces the person's
// email for the competition, waiting to be confirmed, unless it's the same
// address, which keeps its confirmation (and the token is ""). A push
// subscription replaces one with the same endpoint.
func (s *Store) Subscribe(ctx context.Context, sub Subscription) (Subscription, string, error) {
	sub.Address = strings.TrimSpace(sub.Address)
	topics := cleanTopics(sub.Topics)
	sub.Topics = nil
	if topics != "" {
		sub.Topics = strings.Split(topics, ",")
	}
	token := ""
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if sub.Channel == ByEmail {
			var id, address string
			var confirmed bool
			err := tx.QueryRowContext(ctx, `SELECT id, address, confirmed FROM notify_subscriptions WHERE competition_id = $1 AND kind = $2 AND owner_id = $3 AND channel = $4`,
				sub.CompetitionID, sub.Kind, sub.OwnerID, ByEmail).Scan(&id, &address, &confirmed)
			switch {
			case err == nil && strings.EqualFold(address, sub.Address):
				sub.ID, sub.Confirmed = id, confirmed
				_, err := tx.ExecContext(ctx, `UPDATE notify_subscriptions SET topics = $1, page = $2 WHERE id = $3`, topics, sub.Page, id)
				if err != nil || confirmed {
					return err
				}
				// Not confirmed yet: a new link, so the old one stops working.
				token = newToken()
				sub.Token = token
				_, err = tx.ExecContext(ctx, `UPDATE notify_subscriptions SET token = $1 WHERE id = $2`, token, id)
				return err
			case err == nil:
				if _, err := tx.ExecContext(ctx, `DELETE FROM notify_subscriptions WHERE id = $1`, id); err != nil {
					return err
				}
			case !errors.Is(err, sql.ErrNoRows):
				return err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `DELETE FROM notify_subscriptions WHERE competition_id = $1 AND channel = $2 AND address = $3`,
				sub.CompetitionID, ByPush, sub.Address); err != nil {
				return err
			}
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM notify_subscriptions WHERE competition_id = $1 AND kind = $2 AND owner_id = $3 AND channel = $4`,
				sub.CompetitionID, sub.Kind, sub.OwnerID, ByPush).Scan(&n); err != nil {
				return err
			}
			if n >= MaxPush {
				return ErrLimit
			}
			sub.Confirmed = true
		}
		sub.ID, sub.CreatedAt, token = newID(), parseTime(s.stamp()), newToken()
		sub.Token = token
		_, err := tx.ExecContext(ctx, `INSERT INTO notify_subscriptions (id, competition_id, kind, owner_id, channel, address, keys, page, topics, token, confirmed, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			sub.ID, sub.CompetitionID, sub.Kind, sub.OwnerID, sub.Channel, sub.Address, sub.Keys, sub.Page, topics, token, sub.Confirmed, s.stamp())
		return err
	})
	if err != nil {
		return Subscription{}, "", err
	}
	return sub, token, nil
}

// Subscriptions are how one person is told about a competition.
func (s *Store) Subscriptions(ctx context.Context, competitionID, kind, ownerID string) ([]Subscription, error) {
	return s.subscriptions(ctx, `competition_id = $1 AND kind = $2 AND owner_id = $3`, competitionID, kind, ownerID)
}

// CompetitionSubscriptions are everyone told about a competition: push,
// and confirmed emails.
func (s *Store) CompetitionSubscriptions(ctx context.Context, competitionID string) ([]Subscription, error) {
	return s.subscriptions(ctx, `competition_id = $1 AND confirmed`, competitionID)
}

// SubscriptionByToken is the subscription a confirm or off link is for.
func (s *Store) SubscriptionByToken(ctx context.Context, token string) (Subscription, error) {
	return scanSubscription(s.db.QueryRowContext(ctx, `SELECT `+subscriptionColumns+` FROM notify_subscriptions WHERE token = $1`, token))
}

// ConfirmSubscription confirms an email by its link's token.
func (s *Store) ConfirmSubscription(ctx context.Context, token string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE notify_subscriptions SET confirmed = TRUE WHERE token = $1`, token))
}

// Unsubscribe turns a subscription off by its link's token.
func (s *Store) Unsubscribe(ctx context.Context, token string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM notify_subscriptions WHERE token = $1`, token))
}

// RemoveSubscription turns one of a person's subscriptions off, from their page.
func (s *Store) RemoveSubscription(ctx context.Context, competitionID, kind, ownerID, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM notify_subscriptions WHERE competition_id = $1 AND kind = $2 AND owner_id = $3 AND id = $4`,
		competitionID, kind, ownerID, id))
}

// DropSubscription removes a subscription the push service says is gone.
func (s *Store) DropSubscription(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM notify_subscriptions WHERE id = $1`, id)
	return err
}

// MarkChanged records, before a change that can notify, what was live as the
// competition's baseline and when telling is due, unless changes are
// already waiting: then this one joins them.
func (s *Store) MarkChanged(ctx context.Context, competitionID string, baseline []byte, due time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE competitions SET notify_baseline = $1, notify_due = $2 WHERE id = $3 AND notify_due = ''`,
		string(baseline), formatTime(due), competitionID)
	return err
}

// NotifyNow makes the changes waiting due now.
func (s *Store) NotifyNow(ctx context.Context, competitionID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE competitions SET notify_due = $1 WHERE id = $2 AND notify_due <> ''`, s.stamp(), competitionID)
	return err
}

// SetNoWait turns the wait off on the competition's days, or back on.
func (s *Store) SetNoWait(ctx context.Context, competitionID string, on bool) error {
	return affected(s.db.ExecContext(ctx, `UPDATE competitions SET notify_no_wait = $1 WHERE id = $2`, on, competitionID))
}

// Due is a competition whose waiting changes are due, with its baseline.
type Due struct {
	CompetitionID string
	Baseline      []byte
}

// ClaimDue takes the competitions whose changes are due at now, clearing
// their baselines so changes made from now on start a new wait.
func (s *Store) ClaimDue(ctx context.Context) ([]Due, error) {
	var out []Due
	err := s.tx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id, notify_baseline FROM competitions WHERE notify_due <> '' AND notify_due <= $1`, s.stamp())
		if err != nil {
			return err
		}
		for rows.Next() {
			var d Due
			var baseline string
			if err := rows.Scan(&d.CompetitionID, &baseline); err != nil {
				rows.Close()
				return err
			}
			d.Baseline = []byte(baseline)
			out = append(out, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, d := range out {
			if _, err := tx.ExecContext(ctx, `UPDATE competitions SET notify_baseline = '', notify_due = '' WHERE id = $1`, d.CompetitionID); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}
