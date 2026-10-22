package web

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The club admin's roles and links (ADR 0015), on the club's Settings: a
// role is a name and the permissions ticked; a link is a person's name and
// their role, shown once.

const (
	// maxRoleName and maxLinkName are how long a role's and a link's names
	// can be, in characters.
	maxRoleName = 40
	maxLinkName = 60
	// clubHistoryShown is how many changes Settings shows before "See every
	// change kept".
	clubHistoryShown = 50
)

func (p *competitionPages) registerClubRoles(handle func(string, http.HandlerFunc)) {
	handle("POST "+clubPrefix+"/roles", p.addClubRole)
	handle("POST "+clubPrefix+"/roles/{role}", p.saveClubRole)
	handle("POST "+clubPrefix+"/roles/{role}/remove", p.removeClubRole)
	handle("POST "+clubPrefix+"/links", p.addClubLink)
	handle("POST "+clubPrefix+"/links/{link}/role", p.moveClubLink)
	handle("POST "+clubPrefix+"/links/{link}/remove", p.removeClubLink)
}

// backToClubSettings goes back to a part of the club's Settings (at, its
// id), saying what happened.
func backToClubSettings(w http.ResponseWriter, r *http.Request, at, notice string) {
	seeOther(w, r, clubSettingsPath(r.PathValue("token"))+"#"+at, notice)
}

// roleNamed is the role name posted (name), or why it won't do: none, too
// long, or another of the club's roles' (but except's, the role being
// changed).
func roleNamed(r *http.Request, roles []store.ClubRole, except string) (string, string) {
	name := strings.Join(strings.Fields(r.FormValue("name")), " ")
	switch {
	case name == "" || len([]rune(name)) > maxRoleName:
		return "", fmt.Sprintf("Give the role a name, up to %d letters, e.g. Treasurer; nothing was changed.", maxRoleName)
	case slices.ContainsFunc(roles, func(role store.ClubRole) bool { return role.ID != except && strings.EqualFold(role.Name, name) }):
		return "", "There's already a role called " + name + "; nothing was changed. Give this one another name."
	}
	return name, ""
}

// addClubRole makes a role (name, and permission for each ticked).
func (p *competitionPages) addClubRole(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	roles, err := p.st.ClubRoles(r.Context(), club.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	name, problem := roleNamed(r, roles, "")
	if problem != "" {
		backToClubSettings(w, r, "roles", problem)
		return
	}
	if _, err := p.st.AddClubRole(r.Context(), club.ID, name, r.Form["permission"]); errors.Is(err, store.ErrLimit) {
		backToClubSettings(w, r, "roles", fmt.Sprintf("A club can have %d roles: remove one first; nothing was changed.", store.MaxClubRoles))
		return
	} else if err != nil {
		failed(w, r, err)
		return
	}
	backToClubSettings(w, r, "links", "Made the role "+name+". Now make a link with it for each person, under Links.")
}

// saveClubRole renames a role and sets its permissions, for every link with
// it at once.
func (p *competitionPages) saveClubRole(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	roles, err := p.st.ClubRoles(r.Context(), club.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	name, problem := roleNamed(r, roles, r.PathValue("role"))
	if problem != "" {
		backToClubSettings(w, r, "roles", problem)
		return
	}
	if err := p.st.SaveClubRole(r.Context(), club.ID, r.PathValue("role"), name, r.Form["permission"]); errors.Is(err, store.ErrNotFound) {
		backToClubSettings(w, r, "roles", "That role was removed already; nothing was changed.")
		return
	} else if err != nil {
		failed(w, r, err)
		return
	}
	backToClubSettings(w, r, "roles", "Saved "+name+": every link with it can do this now.")
}

// removeClubRole removes a role without links.
func (p *competitionPages) removeClubRole(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	roles, err := p.st.ClubRoles(r.Context(), club.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	i := slices.IndexFunc(roles, func(role store.ClubRole) bool { return role.ID == r.PathValue("role") })
	if i < 0 {
		backToClubSettings(w, r, "roles", "That role was removed already; nothing was changed.")
		return
	}
	switch err := p.st.RemoveClubRole(r.Context(), club.ID, roles[i].ID); {
	case errors.Is(err, store.ErrInUse):
		backToClubSettings(w, r, "roles", fmt.Sprintf("%s still has %s: give them another role or remove them first; nothing was changed.",
			roles[i].Name, views.Count(roles[i].Links, "link", "links")))
		return
	case errors.Is(err, store.ErrNotFound):
		backToClubSettings(w, r, "roles", "That role was removed already; nothing was changed.")
		return
	case err != nil:
		failed(w, r, err)
		return
	}
	backToClubSettings(w, r, "roles", "Removed the role "+roles[i].Name+".")
}

// addClubLink makes a link for a person (name) with a role (role), and
// shows it, once.
func (p *competitionPages) addClubLink(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	name := strings.Join(strings.Fields(r.FormValue("name")), " ")
	if name == "" || len([]rune(name)) > maxLinkName {
		backToClubSettings(w, r, "links", fmt.Sprintf("Say who the link is for, up to %d letters; nothing was changed.", maxLinkName))
		return
	}
	// A name names one link, so a reload of the page showing a new link
	// doesn't make another.
	links, err := p.st.ClubLinks(r.Context(), club.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	if slices.ContainsFunc(links, func(l store.ClubLink) bool { return strings.EqualFold(l.Name, name) }) {
		backToClubSettings(w, r, "links", "There's already a link for "+name+"; nothing was changed. To give them a new one, remove that one first.")
		return
	}
	l, token, err := p.st.AddClubLink(r.Context(), club.ID, r.FormValue("role"), name)
	switch {
	case errors.Is(err, store.ErrLimit):
		backToClubSettings(w, r, "links", fmt.Sprintf("A club can have %d links: remove one first; nothing was changed.", store.MaxClubLinks))
		return
	case errors.Is(err, store.ErrNotFound):
		backToClubSettings(w, r, "links", "Choose a role for the link; nothing was changed.")
		return
	case err != nil:
		failed(w, r, err)
		return
	}
	// Shown on this response only: it isn't kept, so it can't go in a redirect's URL.
	p.renderClub(w, r, club, clubSettings, &views.MemberLink{Name: l.Name, Role: l.Role, Link: origin(r) + clubPath(token)})
}

// moveClubLink gives a link another role (role), at once.
func (p *competitionPages) moveClubLink(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	switch err := p.st.SetClubLinkRole(r.Context(), club.ID, r.PathValue("link"), r.FormValue("role")); {
	case errors.Is(err, store.ErrNotFound):
		backToClubSettings(w, r, "links", "That link or role was removed; nothing was changed.")
		return
	case err != nil:
		failed(w, r, err)
		return
	}
	links, err := p.st.ClubLinks(r.Context(), club.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	notice := "Changed the link's role."
	if i := slices.IndexFunc(links, func(l store.ClubLink) bool { return l.ID == r.PathValue("link") }); i >= 0 {
		notice = links[i].Name + "'s link is " + links[i].Role + " now."
	}
	backToClubSettings(w, r, "links", notice)
}

// removeClubLink stops a link with a role working, asking first.
func (p *competitionPages) removeClubLink(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	links, err := p.st.ClubLinks(r.Context(), club.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	i := slices.IndexFunc(links, func(l store.ClubLink) bool { return l.ID == r.PathValue("link") })
	if i < 0 {
		backToClubSettings(w, r, "links", "That link was removed already; nothing was changed.")
		return
	}
	if p.askClub(w, r, club, clubSettings, views.Ask{
		Remove: true, // takes something away: red
		Title:  "Remove " + links[i].Name + "'s link?",
		Text:   "It stops working at once, wherever it's saved. To let them in again, make a new link and send it to them.",
		Button: "Remove the link", Back: clubSettingsPath(r.PathValue("token")) + "#links",
	}) {
		return
	}
	if err := p.st.RemoveClubLink(r.Context(), club.ID, links[i].ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		failed(w, r, err)
		return
	}
	backToClubSettings(w, r, "links", links[i].Name+"'s link has stopped working.")
}

// clubRolesPart fills in Settings' roles and links, for the club admin, and
// the club's history.
func (p *competitionPages) clubRolesPart(r *http.Request, club store.Club, page *views.ClubPage) error {
	ctx := r.Context()
	if clubLinkOf(r).Admin() {
		roles, err := p.st.ClubRoles(ctx, club.ID)
		if err != nil {
			return err
		}
		for _, role := range roles {
			page.Roles = append(page.Roles, views.ClubRoleView{ID: role.ID, Name: role.Name, Permissions: role.Permissions, Links: role.Links})
		}
		links, err := p.st.ClubLinks(ctx, club.ID)
		if err != nil {
			return err
		}
		for _, l := range links {
			page.Links = append(page.Links, views.ClubLinkView{ID: l.ID, Name: l.Name, RoleID: l.RoleID, Role: l.Role, Made: l.CreatedAt.In(local).Format("2 January")})
		}
	}
	page.AllHistory = r.URL.Query().Get("history") == "all"
	limit := clubHistoryShown + 1 // one more, to know there are more
	if page.AllHistory {
		limit = store.MaxClubHistory
	}
	changes, err := p.st.ClubHistory(ctx, club.ID, limit)
	if err != nil {
		return err
	}
	if !page.AllHistory && len(changes) > clubHistoryShown {
		changes = changes[:clubHistoryShown]
		page.MoreHistory = clubSettingsPath(r.PathValue("token")) + "?history=all#history"
	}
	for _, ch := range changes {
		page.History = append(page.History, views.HistoryChange{At: ch.At.In(local).Format("Mon 2 Jan, 15:04"), Who: ch.Who, What: ch.What})
	}
	return nil
}
