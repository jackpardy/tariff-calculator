package views

import "context"

// ClubAccess is what the club link a page was opened by can do (ADR 0015):
// the club admin's does everything, a link with a role what the role
// allows, so the page offers only that.
type ClubAccess struct {
	Who          string // the link's, e.g. "Mary (Treasurer)"; "Club admin" for the admin link
	Admin        bool   // the club admin: roles and links, replacing the admin link, deleting the club
	Enter        bool   // entering members, changing and withdrawing entries, offers, late changes
	Send         bool   // sending the entries and coaches
	Fees         bool   // each competition's fees and invoice
	Members      bool   // the join link; removing and re-linking members, choosing their coach, judging qualifications
	Coaches      bool   // adding, removing and re-linking coaches, who signs off, their qualifications
	Certificates bool   // opening coaches' and judges' certificates
	Organiser    bool   // telling the organiser, hearing about changes
	Settings     bool   // the club's settings and history
}

// clubAdminAccess is what a page shows outside a club link's gate: all of it.
var clubAdminAccess = ClubAccess{Who: "Club admin", Admin: true, Enter: true, Send: true, Fees: true, Members: true, Coaches: true,
	Certificates: true, Organiser: true, Settings: true}

type clubAccessKey struct{}

// WithClubAccess keeps what the page's club link can do, for its views.
func WithClubAccess(ctx context.Context, a ClubAccess) context.Context {
	return context.WithValue(ctx, clubAccessKey{}, a)
}

// ClubAccessOf is what the page's club link can do: everything, for a page
// not opened by one.
func ClubAccessOf(ctx context.Context) ClubAccess {
	if a, ok := ctx.Value(clubAccessKey{}).(ClubAccess); ok {
		return a
	}
	return clubAdminAccess
}
