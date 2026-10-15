package web

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// One link for a person with several roles (ADR 0010): whichever of their
// personal links someone opens, tabs above the page switch between their
// roles. Tokens are only kept as hashes, so a page can't link to another of
// the person's links: their other roles open at their own paths with
// store.Via(personal link, role's id) as the token, which every page's lookup
// accepts. They add their other links on their details, and take them off
// there; a comp sec makes a member a coach too.

func (p *competitionPages) registerRoles(handle func(string, http.HandlerFunc)) {
	for _, base := range []string{"/clubs/member/{token}", "/clubs/coach/{token}", "/competitions/gymnast/{token}"} {
		handle("POST "+base+"/links", p.addOtherLink)
		handle("POST "+base+"/links/remove", p.removeOtherLink)
	}
}

// personalPrefixes are the paths of pages a personal link opens.
var personalPrefixes = []string{"/clubs/member/", "/clubs/coach/", "/competitions/entry/", "/competitions/gymnast/"}

// withRoles keeps the person's roles for the page a personal link (or a Via
// through one) opens, so its layout shows their tabs. A shared link has no
// person, and no tabs.
func (p *competitionPages) withRoles(r *http.Request) *http.Request {
	token := r.PathValue("token")
	if token == "" || r.Method != http.MethodGet {
		return r
	}
	if !store.IsVia(token) && !slices.ContainsFunc(personalPrefixes, func(prefix string) bool { return strings.HasPrefix(r.URL.Path, prefix) }) {
		return r
	}
	tabs := p.roleTabs(r, token)
	if len(tabs) < 2 {
		return r
	}
	return r.WithContext(views.WithRoles(r.Context(), tabs))
}

// roleTabs are a person's roles as tabs, with the one the page is in current.
// Entering on their own is one tab, however many links they had for it.
func (p *competitionPages) roleTabs(r *http.Request, token string) []views.RoleTab {
	h, roles, err := p.st.Roles(r.Context(), token)
	if err != nil {
		return nil
	}
	personal := store.Personal(token)
	places := map[string]bool{}
	for _, role := range roles {
		switch role.Kind {
		case store.RoleMember, store.RoleCoach, store.RoleClub:
			places[role.Place] = true
		}
	}
	var tabs []views.RoleTab
	individual := false
	for _, role := range roles {
		if role.Kind == store.RoleIndividual {
			if individual {
				continue
			}
			individual = true
		}
		href := roleHref(personal, h, role)
		label := roleLabel(role)
		if len(places) > 1 && role.Place != "" {
			label += " · " + role.Place
		}
		current := r.URL.Path == href || strings.HasPrefix(r.URL.Path, href+"/")
		if role.Kind == store.RoleIndividual {
			current = strings.HasPrefix(r.URL.Path, "/competitions/gymnast/") || strings.HasPrefix(r.URL.Path, "/competitions/entry/")
		}
		tabs = append(tabs, views.RoleTab{Label: label, Href: href, Current: current})
	}
	return tabs
}

// roleHref is where a personal link opens one of the person's roles.
func roleHref(personal string, h store.Holder, role store.Role) string {
	via := store.Via(personal, role.ID)
	switch role.Kind {
	case store.RoleMember:
		if role.Own {
			return memberPath(personal)
		}
		return memberPath(via)
	case store.RoleCoach:
		if role.Own {
			return coachPath(personal)
		}
		return coachPath(via)
	case store.RoleIndividual:
		if h.Kind == store.RoleIndividual {
			return gymnastPath(personal)
		}
		return gymnastPath(via)
	case store.RoleClub:
		return clubPath(via)
	}
	return adminPath(via)
}

// roleLabel is a role's tab: short, for a phone.
func roleLabel(role store.Role) string {
	switch role.Kind {
	case store.RoleMember:
		return "Member"
	case store.RoleCoach:
		return "Coach"
	case store.RoleClub:
		return "Competition secretary"
	case store.RoleIndividual:
		return "On your own"
	case store.RoleOrganiser:
		return "Organiser · " + role.Place
	}
	return role.Name + " · " + role.Place
}

// roleDescription is a role in words, for the list on the person's details.
func roleDescription(role store.Role) string {
	switch role.Kind {
	case store.RoleMember:
		return "Member of " + role.Place + ", as " + role.Name
	case store.RoleCoach:
		return "Coach at " + role.Place + ", as " + role.Name
	case store.RoleClub:
		return "Competition secretary of " + role.Place
	case store.RoleIndividual:
		return "Entering on your own, as " + role.Name
	case store.RoleOrganiser:
		return "Organiser of " + role.Place
	}
	return role.Name + " at " + role.Place
}

// otherLinks are the person's other roles, for their details, posting to
// base's links. They're on the page of the link the person has, so nil on
// one opened through another of their links, where "other" would be unclear.
func (p *competitionPages) otherLinks(r *http.Request, base string) (*views.OtherLinks, error) {
	if store.IsVia(r.PathValue("token")) {
		return nil, nil
	}
	o := &views.OtherLinks{Add: base + "/links", Remove: base + "/links/remove"}
	_, roles, err := p.st.Roles(r.Context(), r.PathValue("token"))
	for _, role := range roles {
		if role.Own {
			continue
		}
		id := role.ID
		if role.TieID != "" {
			id = role.TieID
		}
		o.Roles = append(o.Roles, views.OtherRole{Label: roleDescription(role), Kind: role.Kind, ID: id})
	}
	return o, err
}

// detailsOf is where a personal page's details are, which the other links
// box is on, for a path under its base.
func detailsOf(r *http.Request) string {
	token := r.PathValue("token")
	switch {
	case strings.HasPrefix(r.URL.Path, "/clubs/member/"):
		return memberDetailsPath(token)
	case strings.HasPrefix(r.URL.Path, "/clubs/coach/"):
		return coachPath(token)
	}
	return gymnastDetailsPath(token)
}

// The links someone can paste, by where they lead: the role they are, or ""
// for a link that isn't anyone's own (joining a club, entering a
// competition, a synchro partner's or a coach's sign-off link).
var pastedPaths = []struct{ prefix, kind string }{
	{"/clubs/member/", store.RoleMember},
	{"/clubs/coach/", store.RoleCoach},
	{"/competitions/gymnast/", store.RoleIndividual},
	{"/competitions/entry/", store.RoleIndividual},
	{"/clubs/admin/", store.RoleClub},
	{"/competitions/admin/", store.RoleOrganiser},
	{"/clubs/join/", ""},
	{"/competitions/club/", ""},
	{"/competitions/enter/", ""},
	{"/competitions/partner/", ""},
	{"/competitions/signoff/", ""},
}

// pastedLink is which of the site's links someone pasted: its kind, as
// pastedPaths says, and its token; ok is false for none of them.
func pastedLink(link string) (kind, token string, ok bool) {
	link = strings.TrimSpace(link)
	for _, p := range pastedPaths {
		i := strings.LastIndex(link, p.prefix)
		if i < 0 {
			continue
		}
		t := link[i+len(p.prefix):]
		if j := strings.IndexAny(t, "/?#"); j >= 0 {
			t = t[:j]
		}
		if u, err := url.PathUnescape(t); err == nil {
			t = u
		}
		if t != "" {
			return p.kind, t, true
		}
	}
	return "", "", false
}

// addOtherLink adds a pasted link to the person's page: pasting it proves
// they hold it, so there's nothing to approve.
func (p *competitionPages) addOtherLink(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if _, err := p.st.HolderOf(r.Context(), token); err != nil {
		failed(w, r, err)
		return
	}
	var notice string
	kind, pasted, ok := pastedLink(r.FormValue("link"))
	switch {
	case !ok:
		notice = "That isn't a link from this site. Copy the whole link, from the address bar of the page it opens."
	case kind == "":
		notice = "That's a link for joining a club or entering a competition, which anyone can be sent, not one of your own. Add the link of the page it gave you."
	default:
		role, err := p.st.Tie(r.Context(), token, kind, pasted)
		switch {
		case errors.Is(err, store.ErrOwnLink):
			notice = "That's this page's own link: nothing to add."
		case errors.Is(err, store.ErrTied):
			notice = "That link is already on your page."
		case errors.Is(err, store.ErrNotFound):
			notice = "That link doesn't lead anywhere: it may have been replaced. Check you copied all of it."
		case err != nil:
			failed(w, r, err)
			return
		case role.Place != "":
			notice = "Added: " + roleDescription(role) + ". Use the tabs at the top to switch."
		default:
			notice = "Added your other link. Use the tabs at the top to switch."
		}
	}
	seeOther(w, r, detailsOf(r)+"#other-links", notice)
}

// removeOtherLink takes one of the person's other roles off their page.
func (p *competitionPages) removeOtherLink(w http.ResponseWriter, r *http.Request) {
	notice := "Taken off your page."
	switch err := p.st.Untie(r.Context(), r.PathValue("token"), r.FormValue("kind"), r.FormValue("id")); {
	case errors.Is(err, store.ErrOwnLink):
		notice = "That's this page's own link: it can't be taken off."
	case errors.Is(err, store.ErrNotFound):
		notice = "That wasn't on your page."
	case err != nil:
		failed(w, r, err)
		return
	}
	seeOther(w, r, detailsOf(r)+"#other-links", notice)
}

// makeCoach makes one of the club's members a coach too: their member link
// opens the coach's page.
func (p *competitionPages) makeCoach(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	coach, err := p.st.MakeCoach(r.Context(), club.ID, r.FormValue("member"))
	notice := coach.Name + " is a coach too: their member link opens the coach's page, with a tab for each."
	switch {
	case errors.Is(err, store.ErrTied):
		notice = "They're a coach already."
	case errors.Is(err, store.ErrLimit):
		notice = "The club has as many coaches as it can: remove one first."
	case err != nil:
		failed(w, r, err)
		return
	}
	seeOther(w, r, clubCoachesPath(r.PathValue("token")), notice)
}

// removeClubTie stops someone's own link opening the comp sec's pages.
func (p *competitionPages) removeClubTie(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	notice := "Removed: their own link no longer opens these pages."
	if err := p.st.RemoveClubTie(r.Context(), club.ID, r.FormValue("id")); errors.Is(err, store.ErrNotFound) {
		notice = "That was removed already."
	} else if err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, clubSettingsPath(r.PathValue("token"))+"#ties", notice)
}
