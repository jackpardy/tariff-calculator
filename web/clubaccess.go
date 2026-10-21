package web

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Club roles (ADR 0015): the club's admin link is the club admin's and does
// everything; each link the club admin makes has a role, which allows some
// of the permissions below. Every route under a club's link goes through
// one gate, which checks the link's role against clubRoutes and records
// each change in the club's history.

const clubPrefix = "/clubs/admin/{token}"

// What a route under a club link needs, besides a permission.
const (
	seeClub   = ""      // every role: seeing the club
	adminOnly = "admin" // the club admin alone
)

// clubRoutes say what each route under a club link needs, by its method and
// path after the link ("GET /" for the club's home): seeClub for every
// role, adminOnly for the club admin alone, or a permission. A route that
// isn't here is the club admin's alone (and TestEveryClubRouteHasAPermission
// fails).
var clubRoutes = map[string]string{
	// See the club: its competitions and each one's entries, the members and
	// coaches and each member's routines and skills (looking), timetables
	// and calendars; Feedback.
	"GET /":                                               seeClub,
	"GET /members":                                        seeClub,
	"GET /coaches":                                        seeClub,
	"GET /competitions/{id}":                              seeClub,
	"GET /competitions/{id}/coaches":                      seeClub,
	"GET /competitions/{id}/offers":                       seeClub,
	"GET /competitions/{id}/timeline":                     seeClub,
	"GET /competitions/{id}/calendar.ics":                 seeClub,
	"GET /members/{member}/routines":                      seeClub,
	"GET /members/{member}/skills":                        seeClub,
	"POST /feedback":                                      seeClub,
	"GET /competitions/{id}/enter-member":                 store.ClubEnter,
	"GET /members/{member}/competitions/{id}":             store.ClubEnter,
	"POST /members/{member}/competitions/{id}":            store.ClubEnter,
	"POST /members/{member}/competitions/{id}/withdraw":   store.ClubEnter,
	"GET /competitions/{id}/add-offer":                    store.ClubEnter,
	"GET /members/{member}/competitions/{id}/offer":       store.ClubEnter,
	"POST /members/{member}/competitions/{id}/offer":      store.ClubEnter,
	"GET /members/{member}/competitions/{id}/late":        store.ClubEnter,
	"POST /members/{member}/competitions/{id}/late":       store.ClubEnter,
	"POST /competitions/{id}/send":                        store.ClubSend,
	"POST /competitions/{id}/coaches":                     store.ClubSend,
	"GET /competitions/{id}/fees":                         store.ClubFees,
	"GET /competitions/{id}/invoice":                      store.ClubFees,
	"POST /members/{member}/new-link":                     store.ClubMembers,
	"POST /members/{member}/remove":                       store.ClubMembers,
	"POST /members/{member}/coach":                        store.ClubMembers,
	"POST /members/{member}/judging":                      store.ClubMembers,
	"POST /members/{member}/judging/{q}/remove":           store.ClubMembers,
	"POST /coaches":                                       store.ClubCoaches,
	"POST /coaches/from-member":                           store.ClubCoaches,
	"POST /coaches/{coach}/new-link":                      store.ClubCoaches,
	"POST /coaches/{coach}/remove":                        store.ClubCoaches,
	"POST /coaches/{coach}/signs-off":                     store.ClubCoaches,
	"POST /coaches/{coach}/qualifications":                store.ClubCoaches,
	"POST /coaches/{coach}/qualifications/{q}/remove":     store.ClubCoaches,
	"GET /coaches/{coach}/qualifications/{q}/certificate": store.ClubCertificates,
	"GET /members/{member}/judging/{q}":                   store.ClubCertificates,
	"POST /competitions/{id}/organiser":                   store.ClubOrganiser,
	"POST /competitions/{id}/organiser/{message}":         store.ClubOrganiser,
	"GET /competitions/{id}/notify":                       store.ClubOrganiser,
	"POST /competitions/{id}/notify":                      store.ClubOrganiser,
	"GET /settings":                                       store.ClubSettings,
	"POST /coaches-see-all":                               store.ClubSettings,
	"POST /skills-open":                                   store.ClubSettings,
	"POST /roles":                                         adminOnly,
	"POST /roles/{role}":                                  adminOnly,
	"POST /roles/{role}/remove":                           adminOnly,
	"POST /links":                                         adminOnly,
	"POST /links/{link}/role":                             adminOnly,
	"POST /links/{link}/remove":                           adminOnly,
	"POST /ties/remove":                                   adminOnly,
	"POST /replace-link":                                  adminOnly,
	"POST /delete":                                        adminOnly,
}

// clubRoute is a route's key in clubRoutes, by its pattern, e.g. "POST
// /clubs/admin/{token}/send" is "POST /send"; ok is false for a route not
// under a club link.
func clubRoute(pattern string) (string, bool) {
	method, path, _ := strings.Cut(pattern, " ")
	rest, ok := strings.CutPrefix(path, clubPrefix)
	if !ok || rest != "" && !strings.HasPrefix(rest, "/") {
		return "", false
	}
	if rest == "" {
		rest = "/"
	}
	return method + " " + rest, true
}

// clubAllowed says whether a club link can use a route, by its pattern.
func clubAllowed(l store.ClubLink, pattern string) bool {
	if l.Admin() {
		return true
	}
	key, _ := clubRoute(pattern)
	need, ok := clubRoutes[key]
	switch {
	case !ok || need == adminOnly:
		return false
	case need == seeClub:
		return true
	}
	return l.Can(need)
}

// clubKey is the context key of the club and link a club request came by.
type clubKey struct{}

// clubAt is the club a request's link opens, and which link it is.
type clubAt struct {
	club store.Club
	link store.ClubLink
}

// clubLinkOf is the club link a request came by (the club admin's, outside
// the gate).
func clubLinkOf(r *http.Request) store.ClubLink {
	if at, ok := r.Context().Value(clubKey{}).(clubAt); ok {
		return at.link
	}
	return store.ClubAdmin
}

// clubCan says whether the request's club link has a permission.
func clubCan(r *http.Request, permission string) bool {
	return clubLinkOf(r).Can(permission)
}

// clubAccess is what a club page's controls may offer, for a link.
func clubAccess(l store.ClubLink) views.ClubAccess {
	return views.ClubAccess{
		Who: l.Who(), Admin: l.Admin(),
		Enter:        l.Can(store.ClubEnter),
		Send:         l.Can(store.ClubSend),
		Fees:         l.Can(store.ClubFees),
		Members:      l.Can(store.ClubMembers),
		Coaches:      l.Can(store.ClubCoaches),
		Certificates: l.Can(store.ClubCertificates),
		Organiser:    l.Can(store.ClubOrganiser),
		Settings:     l.Can(store.ClubSettings),
	}
}

// clubGate lets a request through to a route under a club link only by a
// link whose role can use it, and records each change it makes in the
// club's history. It keeps the route, for the test that every one is in
// clubRoutes.
func (p *competitionPages) clubGate(pattern string, h http.HandlerFunc) http.HandlerFunc {
	p.clubPatterns = append(p.clubPatterns, pattern)
	return func(w http.ResponseWriter, r *http.Request) {
		club, link, err := p.st.ClubByLink(r.Context(), r.PathValue("token"))
		if err != nil {
			failed(w, r, err)
			return
		}
		if !clubAllowed(link, pattern) {
			message(w, r, http.StatusForbidden, "Not with this link", "This link can't do that. Ask your club admin.")
			return
		}
		ctx := context.WithValue(r.Context(), clubKey{}, clubAt{club: club, link: link})
		r = r.WithContext(views.WithClubAccess(ctx, clubAccess(link)))
		if r.Method != http.MethodPost {
			h(w, r)
			return
		}
		r.ParseForm() // for describing it after, whatever the handler reads
		what := p.describeClub(r, club, pattern)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h(rec, r)
		to := strings.ToLower(rec.Header().Get("Location"))
		refused := strings.Contains(to, "nothing+was+changed") || strings.Contains(to, "nothing+was+sent")
		if rec.status < 400 && !refused && !rec.asked && what != "" && !strings.HasSuffix(pattern, "/delete") {
			p.st.RecordClub(context.WithoutCancel(r.Context()), club.ID, link.Who(), what)
		}
	}
}

// describeClub says what a change through a club link does, for the
// club's history; "" for one not worth recording.
func (p *competitionPages) describeClub(r *http.Request, club store.Club, pattern string) string {
	key, _ := clubRoute(pattern)
	path := strings.TrimPrefix(key, "POST ")
	ctx, f := r.Context(), r.Form.Get
	member := func() string {
		if m, err := p.st.Member(ctx, club.ID, r.PathValue("member")); err == nil {
			return m.Name
		}
		return "a member"
	}
	coach := func(id string) string {
		coaches, _ := p.st.Coaches(ctx, club.ID)
		if i := slices.IndexFunc(coaches, func(c store.Coach) bool { return c.ID == id }); i >= 0 {
			return coaches[i].Name
		}
		return "a coach"
	}
	competition := func() string {
		if c, err := p.st.Competition(ctx, r.PathValue("id")); err == nil {
			return c.Name
		}
		return "a competition"
	}
	// entry is the member's entry for the competition in the discipline
	// posted, as the history names it: "Ann Ryan · BUCS L3".
	entry := func() string {
		if e, err := p.memberEntry(r, r.PathValue("member"), r.PathValue("id")); err == nil && e != nil {
			return e.Entry.Gymnasts() + " · " + e.Entry.Event()
		}
		return member() + " · " + competition()
	}
	role := func(id string) string {
		roles, _ := p.st.ClubRoles(ctx, club.ID)
		if i := slices.IndexFunc(roles, func(r store.ClubRole) bool { return r.ID == id }); i >= 0 {
			return roles[i].Name
		}
		return "a role"
	}
	link := func() string {
		links, _ := p.st.ClubLinks(ctx, club.ID)
		if i := slices.IndexFunc(links, func(l store.ClubLink) bool { return l.ID == r.PathValue("link") }); i >= 0 {
			return links[i].Who()
		}
		return "someone"
	}
	switch path {
	case "/competitions/{id}/send":
		if f("which") == "all" {
			return "Sent every entry again to " + competition()
		}
		return "Sent the new and changed entries to " + competition()
	case "/competitions/{id}/coaches":
		return "Sent coaches to " + competition()
	case "/members/{member}/competitions/{id}":
		if e, err := p.memberEntry(r, r.PathValue("member"), r.PathValue("id")); err == nil && e != nil {
			return "Changed an entry: " + entry()
		}
		return "Entered " + member() + " in " + competition()
	case "/members/{member}/competitions/{id}/withdraw":
		if f("confirm") != "1" {
			return ""
		}
		return "Withdrew " + entry()
	case "/members/{member}/competitions/{id}/offer":
		return "Changed " + member() + "'s offer to help at " + competition()
	case "/members/{member}/competitions/{id}/late":
		return "Asked for a late change: " + entry()
	case "/members/{member}/new-link":
		return "Made a new link for " + member()
	case "/members/{member}/remove":
		return "Removed " + member()
	case "/members/{member}/coach":
		return "Chose " + member() + "'s coach"
	case "/members/{member}/judging":
		return "Added a judging qualification for " + member()
	case "/members/{member}/judging/{q}/remove":
		return "Removed a judging qualification of " + member()
	case "/coaches":
		return "Added a coach: " + strings.TrimSpace(f("name"))
	case "/coaches/from-member":
		if m, err := p.st.Member(ctx, club.ID, f("member")); err == nil {
			return "Made " + m.Name + " a coach too"
		}
		return "Made a member a coach too"
	case "/coaches/{coach}/new-link":
		return "Made a new link for " + coach(r.PathValue("coach"))
	case "/coaches/{coach}/remove":
		return "Removed " + coach(r.PathValue("coach"))
	case "/coaches/{coach}/signs-off":
		if f("on") == "1" {
			return coach(r.PathValue("coach")) + " signs off routines"
		}
		return coach(r.PathValue("coach")) + " doesn't sign off routines"
	case "/coaches/{coach}/qualifications":
		return "Added a qualification for " + coach(r.PathValue("coach"))
	case "/coaches/{coach}/qualifications/{q}/remove":
		return "Removed a qualification of " + coach(r.PathValue("coach"))
	case "/coaches-see-all":
		return "Changed who coaches see"
	case "/skills-open":
		return "Changed who sees members' skills"
	case "/competitions/{id}/organiser":
		return "Sent the organiser of " + competition() + " a message"
	case "/competitions/{id}/organiser/{message}":
		return "Added to a message to the organiser of " + competition()
	case "/competitions/{id}/notify":
		return "" // hearing about changes is the person's own
	case "/feedback":
		return "Sent feedback about the app"
	case "/roles":
		return "Made the role " + strings.TrimSpace(f("name"))
	case "/roles/{role}":
		return "Changed the role " + role(r.PathValue("role"))
	case "/roles/{role}/remove":
		return "Removed the role " + role(r.PathValue("role"))
	case "/links":
		return "Made a link for " + strings.TrimSpace(f("name")) + " (" + role(f("role")) + ")"
	case "/links/{link}/role":
		return "Moved " + link() + " to " + role(f("role"))
	case "/links/{link}/remove":
		return "Removed the link for " + link()
	case "/ties/remove":
		return "Stopped someone's own link opening the club admin's pages"
	case "/replace-link":
		return "Replaced the club admin link"
	}
	return "Changed the club's settings"
}
