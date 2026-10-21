package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

// Club roles (ADR 0015). A club's admin link is the club admin's, and does
// everything. The club admin names roles, each a set of the permissions
// below, and makes a link for each person with a role; the link opens the
// club's pages at the same address as the admin link, and does only what
// its role allows. The club keeps a history of what was changed through
// each link.

// What a club role can do, beyond seeing the club (every role sees the
// club's competitions and entries, members and coaches).
const (
	ClubEnter        = "enter"        // enter members, change and withdraw entries, offers, late changes
	ClubSend         = "send"         // send the entries and coaches to the organiser
	ClubFees         = "fees"         // each competition's fees and invoice
	ClubMembers      = "members"      // the join link; remove, re-link and choose members' coaches; judging qualifications
	ClubCoaches      = "coaches"      // add, remove and re-link coaches; who signs off; their qualifications
	ClubCertificates = "certificates" // open coaches' and judges' certificates
	ClubOrganiser    = "organiser"    // tell the organiser, and hear about changes
	ClubSettings     = "settings"     // coaches see everyone, members' skills open; the history
)

// ClubPermissions are the permissions, in the order they're offered.
var ClubPermissions = []string{ClubEnter, ClubSend, ClubFees, ClubMembers, ClubCoaches, ClubCertificates, ClubOrganiser, ClubSettings}

const (
	// MaxClubRoles is how many roles a club can have.
	MaxClubRoles = 10
	// MaxClubLinks is how many links with a role a club can have.
	MaxClubLinks = 30
	// MaxClubHistory is how many changes a club's history keeps.
	MaxClubHistory = 1000
)

// ErrInUse is a role that still has links, so can't be removed.
var ErrInUse = errors.New("the role still has links")

// ClubRole is one of a club's roles.
type ClubRole struct {
	ID          string
	Name        string
	Permissions []string // in ClubPermissions' order
	Links       int      // how many links have it
}

// ClubLink is a link a club's pages were opened by: the club admin's, or a
// person's with a role.
type ClubLink struct {
	ID          string // "" for the club admin's
	Name        string // the person's; "" for the club admin's
	RoleID      string
	Role        string // the role's name
	Permissions []string
	CreatedAt   time.Time
}

// ClubAdmin is the club admin's link, as a ClubLink.
var ClubAdmin = ClubLink{}

// Admin says whether it's the club admin's link.
func (l ClubLink) Admin() bool { return l.ID == "" }

// Can says whether the link can do what a permission allows: the club admin
// can do everything.
func (l ClubLink) Can(permission string) bool {
	return l.Admin() || slices.Contains(l.Permissions, permission)
}

// Who is who the link is, for the history and what's sent from it: "Club
// admin", or the person and their role, e.g. "Mary (Treasurer)".
func (l ClubLink) Who() string {
	if l.Admin() {
		return "Club admin"
	}
	return l.Name + " (" + l.Role + ")"
}

// permissionsOf are the known permissions among those given, in order, as
// kept.
func permissionsOf(given []string) (string, []string) {
	var out []string
	for _, p := range ClubPermissions {
		if slices.Contains(given, p) {
			out = append(out, p)
		}
	}
	b, _ := json.Marshal(out)
	if out == nil {
		b = []byte("[]")
	}
	return string(b), out
}

// ClubByLink is the club a link opens, with which link it is: the club
// admin's, or one with a role. A personal link's Via to the club (for the
// admin's) or to a role's link opens it while the person has that link, as
// it is now, on their page (ADR 0010). The role's permissions are read each
// time, so a change to them counts at once.
func (s *Store) ClubByLink(ctx context.Context, token string) (Club, ClubLink, error) {
	if personal, id, ok := splitVia(token); ok {
		return s.clubVia(ctx, personal, id)
	}
	c, err := s.clubBy(ctx, "admin_hash = $1", hash(token))
	if err == nil {
		return c, ClubAdmin, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Club{}, ClubLink{}, err
	}
	l, clubID, err := s.clubLink(ctx, `l.token_hash = $1`, hash(token))
	if err != nil {
		return Club{}, ClubLink{}, err
	}
	c, err = s.clubBy(ctx, "id = $1", clubID)
	return c, l, err
}

// clubVia is the club link a personal link's Via opens: the club admin's,
// by the club's id, or one with a role, by its id.
func (s *Store) clubVia(ctx context.Context, personal, id string) (Club, ClubLink, error) {
	h, err := holderOf(ctx, s.db, personal)
	if err != nil {
		return Club{}, ClubLink{}, err
	}
	c, err := s.clubBy(ctx, `id = $1 AND EXISTS (SELECT 1 FROM shared_ties t WHERE t.kind = 'club' AND t.target_id = clubs.id
		AND t.link_hash = clubs.admin_hash AND t.record_id IN (`+personRecords("2")+`))`, id, h.PersonID)
	if err == nil {
		return c, ClubAdmin, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Club{}, ClubLink{}, err
	}
	l, clubID, err := s.clubLink(ctx, `l.id = $1 AND EXISTS (SELECT 1 FROM shared_ties t WHERE t.kind = 'clublink' AND t.target_id = l.id
		AND t.link_hash = l.token_hash AND t.record_id IN (`+personRecords("2")+`))`, id, h.PersonID)
	if err != nil {
		return Club{}, ClubLink{}, err
	}
	c, err = s.clubBy(ctx, "id = $1", clubID)
	return c, l, err
}

// clubLink is a link with a role, and its club's id, by a condition on the
// link (l).
func (s *Store) clubLink(ctx context.Context, where string, args ...any) (ClubLink, string, error) {
	var l ClubLink
	var clubID, perms, created string
	err := s.db.QueryRowContext(ctx, `SELECT l.id, l.club_id, l.name, r.id, r.name, r.permissions, l.created_at
		FROM club_links l JOIN club_roles r ON r.id = l.role_id WHERE `+where, args...).
		Scan(&l.ID, &clubID, &l.Name, &l.RoleID, &l.Role, &perms, &created)
	if err != nil {
		return ClubLink{}, "", notFound(err)
	}
	json.Unmarshal([]byte(perms), &l.Permissions)
	l.CreatedAt = parseTime(created)
	return l, clubID, nil
}

// ClubRoles are a club's roles, in the order they were made, with how many
// links each has.
func (s *Store) ClubRoles(ctx context.Context, clubID string) ([]ClubRole, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id, r.name, r.permissions, (SELECT COUNT(*) FROM club_links l WHERE l.role_id = r.id)
		FROM club_roles r WHERE r.club_id = $1 ORDER BY r.position, r.rowid`, clubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClubRole
	for rows.Next() {
		var r ClubRole
		var perms string
		if err := rows.Scan(&r.ID, &r.Name, &perms, &r.Links); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(perms), &r.Permissions)
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddClubRole makes a role, named, with the permissions given (those
// unknown are left out); ErrLimit once the club has MaxClubRoles.
func (s *Store) AddClubRole(ctx context.Context, clubID, name string, permissions []string) (ClubRole, error) {
	kept, perms := permissionsOf(permissions)
	r := ClubRole{ID: newID(), Name: name, Permissions: perms}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var n, last int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(position), 0) FROM club_roles WHERE club_id = $1`, clubID).Scan(&n, &last); err != nil {
			return err
		}
		if n >= MaxClubRoles {
			return ErrLimit
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO club_roles (id, club_id, name, permissions, position) VALUES ($1, $2, $3, $4, $5)`,
			r.ID, clubID, name, kept, last+1)
		return err
	})
	return r, err
}

// SaveClubRole renames a role and sets its permissions, for every link with
// it at once.
func (s *Store) SaveClubRole(ctx context.Context, clubID, id, name string, permissions []string) error {
	kept, _ := permissionsOf(permissions)
	return affected(s.db.ExecContext(ctx, `UPDATE club_roles SET name = $1, permissions = $2 WHERE id = $3 AND club_id = $4`, name, kept, id, clubID))
}

// RemoveClubRole removes a role; ErrInUse while links have it.
func (s *Store) RemoveClubRole(ctx context.Context, clubID, id string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM club_links WHERE role_id = $1`, id).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrInUse
		}
		return affected(tx.ExecContext(ctx, `DELETE FROM club_roles WHERE id = $1 AND club_id = $2`, id, clubID))
	})
}

// AddClubLink makes a link for a person (name) with one of the club's
// roles, returning it and its token, which is kept only as a hash: shown
// once, like the admin link. ErrLimit once the club has MaxClubLinks,
// ErrNotFound for a role that isn't the club's.
func (s *Store) AddClubLink(ctx context.Context, clubID, roleID, name string) (ClubLink, string, error) {
	token := newToken()
	at := s.stamp()
	l := ClubLink{ID: newID(), Name: name, RoleID: roleID, CreatedAt: parseTime(at)}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var perms string
		if err := tx.QueryRowContext(ctx, `SELECT name, permissions FROM club_roles WHERE id = $1 AND club_id = $2`, roleID, clubID).Scan(&l.Role, &perms); err != nil {
			return notFound(err)
		}
		json.Unmarshal([]byte(perms), &l.Permissions)
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM club_links WHERE club_id = $1`, clubID).Scan(&n); err != nil {
			return err
		}
		if n >= MaxClubLinks {
			return ErrLimit
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO club_links (id, club_id, role_id, name, token_hash, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
			l.ID, clubID, roleID, name, hash(token), at)
		return err
	})
	if err != nil {
		return ClubLink{}, "", err
	}
	return l, token, nil
}

// ClubLinks are a club's links with a role, oldest first.
func (s *Store) ClubLinks(ctx context.Context, clubID string) ([]ClubLink, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT l.id, l.name, r.id, r.name, r.permissions, l.created_at
		FROM club_links l JOIN club_roles r ON r.id = l.role_id WHERE l.club_id = $1 ORDER BY l.created_at, l.rowid`, clubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClubLink
	for rows.Next() {
		var l ClubLink
		var perms, created string
		if err := rows.Scan(&l.ID, &l.Name, &l.RoleID, &l.Role, &perms, &created); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(perms), &l.Permissions)
		l.CreatedAt = parseTime(created)
		out = append(out, l)
	}
	return out, rows.Err()
}

// SetClubLinkRole moves a link to another of the club's roles.
func (s *Store) SetClubLinkRole(ctx context.Context, clubID, id, roleID string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE club_links SET role_id = $1 WHERE id = $2 AND club_id = $3
		AND EXISTS (SELECT 1 FROM club_roles WHERE id = $1 AND club_id = $3)`, roleID, id, clubID))
}

// RemoveClubLink stops a link with a role working, at once, and on the
// pages of anyone who added it to theirs.
func (s *Store) RemoveClubLink(ctx context.Context, clubID, id string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if err := affected(tx.ExecContext(ctx, `DELETE FROM club_links WHERE id = $1 AND club_id = $2`, id, clubID)); err != nil {
			return err
		}
		return dropTies(ctx, tx)
	})
}

// RecordClub adds a change to a club's history, keeping the latest
// MaxClubHistory.
func (s *Store) RecordClub(ctx context.Context, clubID, who, what string) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO club_log (club_id, at, who, what) VALUES ($1, $2, $3, $4)`,
		clubID, s.stamp(), who, what); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM club_log WHERE club_id = $1 AND id <= (
		SELECT id FROM club_log WHERE club_id = $1 ORDER BY id DESC LIMIT 1 OFFSET $2)`, clubID, MaxClubHistory)
	return err
}

// ClubHistory is a club's changes, latest first, at most limit.
func (s *Store) ClubHistory(ctx context.Context, clubID string, limit int) ([]Change, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT at, who, what FROM club_log WHERE club_id = $1 ORDER BY id DESC LIMIT $2`, clubID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Change
	for rows.Next() {
		var c Change
		var at string
		if err := rows.Scan(&at, &c.Who, &c.What); err != nil {
			return nil, err
		}
		c.At = parseTime(at)
		out = append(out, c)
	}
	return out, rows.Err()
}
