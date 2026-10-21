package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Feedback about the app (migration 47): anyone on any page can tell the
// maintainer what confused them. It keeps the words, a name or email to reply
// to if given, the role and the page as its route pattern (never a link), the
// competition it was sent from if any, and details from the browser; and,
// as the maintainer chooses, a picture of the screen and a copy of the page,
// kept apart (site_feedback_captures) so they can be deleted on their own.
// The maintainer reads it on a page of their own, through a link only they
// have (NewMaintainerLink).

// SiteFeedback is something someone said about the app.
type SiteFeedback struct {
	ID            string
	Text          string
	ReplyTo       string // a name or email, if they gave one
	Role          string // e.g. "Member", "Organiser", "Calculator"
	Page          string // the route pattern of the page, e.g. "/clubs/member/{token}/competitions/{id}"
	CompetitionID string // the competition it was sent from, "" for none
	Competition   string // its name when it was sent
	Details       string // what the browser said about itself, as JSON
	At            time.Time
	DoneAt        time.Time // zero until it's marked dealt with
	Picture       bool      // a picture of the screen is kept with it
	Snapshot      bool      // a copy of the page is kept with it
}

// Done says whether it's been dealt with.
func (f SiteFeedback) Done() bool { return !f.DoneAt.IsZero() }

// Capture is what's kept with a piece of feedback: a picture of the screen
// (a JPEG, PNG or WebP) and a copy of the page (gzipped HTML), either nil.
type Capture struct {
	Picture     []byte
	PictureType string
	Snapshot    []byte
}

func (c Capture) empty() bool { return len(c.Picture) == 0 && len(c.Snapshot) == 0 }

const (
	// MaxSiteFeedbackPerDay is how much feedback the app takes in 24 hours.
	MaxSiteFeedbackPerDay = 300
	// MaxCapturesBytes is the most the pictures and copies kept may come to:
	// past it, feedback is kept without them.
	MaxCapturesBytes = 500 << 20
)

// AddSiteFeedback keeps feedback and what was captured with it, refusing it
// with ErrLimit once the app has had MaxSiteFeedbackPerDay in the last 24
// hours. The capture is left off, the words still kept, when there's no
// room for it (MaxCapturesBytes, or the disk nearly full).
func (s *Store) AddSiteFeedback(ctx context.Context, f SiteFeedback, c Capture) (SiteFeedback, error) {
	var n int
	since := formatTime(s.now().Add(-24 * time.Hour))
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM site_feedback WHERE at > $1`, since).Scan(&n); err != nil {
		return SiteFeedback{}, err
	}
	if n >= MaxSiteFeedbackPerDay {
		return SiteFeedback{}, ErrLimit
	}
	if !c.empty() {
		used, err := s.CapturesBytes(ctx)
		if err != nil {
			return SiteFeedback{}, err
		}
		free, err := s.free()
		if used+int64(len(c.Picture)+len(c.Snapshot)) > MaxCapturesBytes || err == nil && free < minFreeBytes {
			c = Capture{}
		}
	}
	at := s.stamp()
	f.ID, f.At, f.DoneAt = newID(), parseTime(at), time.Time{}
	f.Picture, f.Snapshot = len(c.Picture) > 0, len(c.Snapshot) > 0
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO site_feedback (id, text, reply_to, role, page, competition_id, competition, details, at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			f.ID, f.Text, f.ReplyTo, f.Role, f.Page, f.CompetitionID, f.Competition, f.Details, at); err != nil {
			return err
		}
		if c.empty() {
			return nil
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO site_feedback_captures (feedback_id, picture, picture_type, snapshot, at) VALUES ($1, $2, $3, $4, $5)`,
			f.ID, nilIfEmpty(c.Picture), c.PictureType, nilIfEmpty(c.Snapshot), at)
		return err
	})
	return f, err
}

func nilIfEmpty(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

// AllSiteFeedback is the app's feedback, newest first, without what was
// captured (SiteFeedbackPicture, SiteFeedbackSnapshot).
func (s *Store) AllSiteFeedback(ctx context.Context) ([]SiteFeedback, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT f.id, f.text, f.reply_to, f.role, f.page, f.competition_id, f.competition, f.details, f.at, f.done_at,
		c.picture IS NOT NULL, c.snapshot IS NOT NULL
		FROM site_feedback f LEFT JOIN site_feedback_captures c ON c.feedback_id = f.id
		ORDER BY f.at DESC, f.rowid DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SiteFeedback
	for rows.Next() {
		var f SiteFeedback
		var at, done string
		var picture, snapshot sql.NullBool
		if err := rows.Scan(&f.ID, &f.Text, &f.ReplyTo, &f.Role, &f.Page, &f.CompetitionID, &f.Competition, &f.Details, &at, &done, &picture, &snapshot); err != nil {
			return nil, err
		}
		f.At, f.DoneAt = parseTime(at), parseTime(done)
		f.Picture, f.Snapshot = picture.Bool, snapshot.Bool
		out = append(out, f)
	}
	return out, rows.Err()
}

// SiteFeedbackPicture is the picture kept with a piece of feedback, and its
// media type; ErrNotFound if there's none.
func (s *Store) SiteFeedbackPicture(ctx context.Context, id string) ([]byte, string, error) {
	var picture []byte
	var kind string
	err := s.db.QueryRowContext(ctx, `SELECT picture, picture_type FROM site_feedback_captures WHERE feedback_id = $1 AND picture IS NOT NULL`, id).Scan(&picture, &kind)
	return picture, kind, notFound(err)
}

// SiteFeedbackSnapshot is the copy of the page kept with a piece of
// feedback, gzipped; ErrNotFound if there's none.
func (s *Store) SiteFeedbackSnapshot(ctx context.Context, id string) ([]byte, error) {
	var snapshot []byte
	err := s.db.QueryRowContext(ctx, `SELECT snapshot FROM site_feedback_captures WHERE feedback_id = $1 AND snapshot IS NOT NULL`, id).Scan(&snapshot)
	return snapshot, notFound(err)
}

// MarkSiteFeedback marks feedback dealt with (done), or not.
func (s *Store) MarkSiteFeedback(ctx context.Context, id string, done bool) error {
	at := ""
	if done {
		at = s.stamp()
	}
	return affected(s.db.ExecContext(ctx, `UPDATE site_feedback SET done_at = $1 WHERE id = $2`, at, id))
}

// CapturesBytes is how much the pictures and copies kept come to.
func (s *Store) CapturesBytes(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(COALESCE(LENGTH(picture), 0) + COALESCE(LENGTH(snapshot), 0)), 0) FROM site_feedback_captures`).Scan(&n)
	return n, err
}

// DeleteCaptures deletes the pictures and copies of pages kept before a
// time, or all of them for the zero time; the words stay. It says how many
// went.
func (s *Store) DeleteCaptures(ctx context.Context, before time.Time) (int64, error) {
	query, args := `DELETE FROM site_feedback_captures`, []any{}
	if !before.IsZero() {
		query, args = query+` WHERE at < $1`, []any{formatTime(before)}
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// FeedbackSettings are the maintainer's choices for feedback about the app.
type FeedbackSettings struct {
	// Off hides the Feedback button on every page. It's on until turned off.
	Off bool
	// Light sends only details about the browser, and a picture only when
	// the person ticks for it, with private parts blurred. Otherwise (Full,
	// for the demo and the dry run) a picture and a copy of the page go with
	// every piece.
	Light bool
}

const (
	settingFeedbackOff   = "feedback_off"
	settingFeedbackLight = "feedback_light"
)

// SiteFeedbackSettings reads the maintainer's choices for feedback.
func (s *Store) SiteFeedbackSettings(ctx context.Context) (FeedbackSettings, error) {
	var out FeedbackSettings
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings WHERE key IN ($1, $2)`, settingFeedbackOff, settingFeedbackLight)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return out, err
		}
		switch key {
		case settingFeedbackOff:
			out.Off = value == "1"
		case settingFeedbackLight:
			out.Light = value == "1"
		}
	}
	return out, rows.Err()
}

// SetSiteFeedbackSettings keeps the maintainer's choices for feedback.
func (s *Store) SetSiteFeedbackSettings(ctx context.Context, f FeedbackSettings) error {
	flag := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		for key, value := range map[string]string{settingFeedbackOff: flag(f.Off), settingFeedbackLight: flag(f.Light)} {
			if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
				return err
			}
		}
		return nil
	})
}

// settingMaintainerLink holds the hash of the maintainer's link.
const settingMaintainerLink = "maintainer_link"

// NewMaintainerLink makes the maintainer's link, for reading feedback about
// the app, and returns its token; a link made before stops working. Only
// its hash is kept.
func (s *Store) NewMaintainerLink(ctx context.Context) (string, error) {
	token := newToken()
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		settingMaintainerLink, hash(token))
	return token, err
}

// IsMaintainerLink says whether a token is the maintainer's link.
func (s *Store) IsMaintainerLink(ctx context.Context, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings WHERE key = $1 AND value = $2`, settingMaintainerLink, hash(token)).Scan(&n)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	return n > 0, nil
}
