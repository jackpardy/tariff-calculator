package web

import (
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The menus of the club pages (roadmap 2026-10-13, finding your way round):
// a comp sec's sections, and a member's tabs. Each page says which section it
// is in; to add a section, add it to the list and give it a page.

// The sections of a comp sec's pages.
const (
	clubCompetitions = "competitions"
	clubMembers      = "members"
	clubCoaches      = "coaches"
	clubSettings     = "settings"
)

// clubMembersPath, clubCoachesPath and clubSettingsPath are the sections of a
// comp sec's pages; the Competitions section is the club's path itself.
func clubMembersPath(token string) string  { return clubPath(token) + "/members" }
func clubCoachesPath(token string) string  { return clubPath(token) + "/coaches" }
func clubSettingsPath(token string) string { return clubPath(token) + "/settings" }

// clubMenu is the menu of a comp sec's pages, with the section shown current.
// The entry-editing pages are in Competitions.
func clubMenu(token, current string) []views.MenuSection {
	sections := []views.MenuSection{
		{Label: "Competitions", Href: clubPath(token), Current: current == clubCompetitions},
		{Label: "Members", Href: clubMembersPath(token), Current: current == clubMembers},
		{Label: "Coaches", Href: clubCoachesPath(token), Current: current == clubCoaches},
		{Label: "Settings", Href: clubSettingsPath(token), Current: current == clubSettings},
	}
	return sections
}

// The tabs of one competition's pages, in the comp sec's Competitions.
const (
	clubCompEntries = "entries"
	clubCompCoaches = "coaches"
	clubCompOffers  = "offers"
	clubCompFees    = "fees"
)

// clubCompetitionPath is the comp sec's page for a competition: its entries.
func clubCompetitionPath(token, competitionID string) string {
	return clubPath(token) + "/competitions/" + competitionID
}

// clubCompetitionMenu is the comp sec's menu on a competition's pages:
// Competitions current, with the competition's own tabs under it. Coaches
// only where it approves coaches, Fees only where it charges.
func clubCompetitionMenu(token string, c store.Competition, current string) []views.MenuSection {
	page := clubCompetitionPath(token, c.ID)
	items := []views.MenuItem{{Label: "Entries", Href: page, Current: current == clubCompEntries}}
	if c.ApproveCoaches && c.Signoff {
		items = append(items, views.MenuItem{Label: "Coaches", Href: page + "/coaches", Current: current == clubCompCoaches})
	}
	items = append(items, views.MenuItem{Label: "Officiating", Href: page + "/offers", Current: current == clubCompOffers})
	if c.Fees.On() {
		items = append(items, views.MenuItem{Label: "Fees", Href: page + "/fees", Current: current == clubCompFees})
	}
	menu := clubMenu(token, clubCompetitions)
	menu[0].Items = items
	return menu
}

// The tabs of a member's pages.
const (
	memberCompetitions    = "competitions"
	memberRoutinesTab     = "routines"
	memberSkillsTab       = "skills"
	memberRequirementsTab = "requirements"
	memberDetails         = "details"
)

// memberDetailsPath is a member's details: their link and their coach.
func memberDetailsPath(token string) string { return memberPath(token) + "/details" }

// memberCompetitionPath is a member's page for a competition.
func memberCompetitionPath(token, competitionID string) string {
	return memberPath(token) + "/competitions/" + competitionID
}

// memberMenu is the menu of a member's pages, with the tab shown current. A
// competition's page is in Competitions.
func memberMenu(token, current string) []views.MenuSection {
	return []views.MenuSection{
		{Label: "Competitions", Href: memberPath(token), Current: current == memberCompetitions},
		{Label: "Routines", Href: memberRoutinesPath(token), Current: current == memberRoutinesTab},
		{Label: "Skills", Href: memberSkillsPath(token), Current: current == memberSkillsTab},
		{Label: "Requirements", Href: memberPath(token) + "/requirements", Current: current == memberRequirementsTab},
		{Label: "Your details", Href: memberDetailsPath(token), Current: current == memberDetails},
	}
}

// The sections of a coach's pages.
const (
	coachEntriesTab      = "entries"
	coachGymnastsTab     = "gymnasts"
	coachClubSkillsTab   = "club-skills"
	coachLevelsTab       = "levels"
	coachRequirementsTab = "requirements"
	coachDetailsTab      = "details"
)

// coachDetailsPath is a coach's details: their link and their other links.
func coachDetailsPath(token string) string { return coachPath(token) + "/details" }

// coachMenu is the menu of a coach's pages, with the section shown current:
// the entries they see (their default page, where signing one off is too),
// the gymnasts they see, with their routines and skills, the club's own
// skills, and the club's levels.
func coachMenu(token, current string) []views.MenuSection {
	menu := []views.MenuSection{
		{Label: "Entries", Href: coachPath(token), Current: current == coachEntriesTab},
		{Label: "Gymnasts", Href: coachGymnastsPath(token), Current: current == coachGymnastsTab},
		{Label: "Club skills", Href: coachPath(token) + "/club-skills", Current: current == coachClubSkillsTab},
		{Label: "Levels", Href: coachLevelsPath(token), Current: current == coachLevelsTab},
		{Label: "Requirements", Href: coachPath(token) + "/requirements", Current: current == coachRequirementsTab},
	}
	// Your details only for the coach's own link: opened through another of
	// their links (a member's who coaches too), that link's page has them.
	if !store.IsVia(token) {
		menu = append(menu, views.MenuSection{Label: "Your details", Href: coachDetailsPath(token), Current: current == coachDetailsTab})
	}
	return menu
}
