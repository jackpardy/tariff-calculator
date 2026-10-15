package web

import (
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

// The tabs of a member's pages.
const (
	memberCompetitions = "competitions"
	memberRoutinesTab  = "routines"
	memberDetails      = "details"
)

// memberDetailsPath is a member's details: their link and their coach.
func memberDetailsPath(token string) string { return memberPath(token) + "/details" }

// memberCompetitionPath is a member's page for a competition.
func memberCompetitionPath(token, competitionID string) string {
	return memberPath(token) + "/competitions/" + competitionID
}

// memberMenu is the menu of a member's pages, with the tab shown current. A
// competition's page is in Competitions. Skills will be another tab here
// (ADR 0011).
func memberMenu(token, current string) []views.MenuSection {
	return []views.MenuSection{
		{Label: "Competitions", Href: memberPath(token), Current: current == memberCompetitions},
		{Label: "Routines", Href: memberRoutinesPath(token), Current: current == memberRoutinesTab},
		{Label: "Your details", Href: memberDetailsPath(token), Current: current == memberDetails},
	}
}

// The sections of a coach's pages.
const (
	coachEntriesTab  = "entries"
	coachGymnastsTab = "gymnasts"
)

// coachMenu is the menu of a coach's pages, with the section shown current:
// the entries they see (their default page, where signing one off is too),
// and the gymnasts they see, with their routines.
func coachMenu(token, current string) []views.MenuSection {
	return []views.MenuSection{
		{Label: "Entries", Href: coachPath(token), Current: current == coachEntriesTab},
		{Label: "Gymnasts", Href: coachGymnastsPath(token), Current: current == coachGymnastsTab},
	}
}
