package web

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/requirements"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Sharing club levels (ADR 0011 Decision 6). A coach's Levels page gives each
// level a link carrying it, with the club's own skills it tracks, in the
// link's fragment, which never reaches the server until someone opens it:
// the page it opens reads the level from the fragment and shows it, and a
// coach of another club adds a copy to theirs, to track against and change.
// No marks or names of people travel with it.

func (p *competitionPages) registerLevelSharing(handle func(string, http.HandlerFunc)) {
	handle("GET /clubs/levels/shared", p.sharedLevelPage)
	handle("POST /clubs/levels/shared", p.sharedLevelPreview)
	handle("POST /clubs/levels/import", p.importSharedLevel)
}

// sharedLevel is a shared club level, as its link carries it.
type sharedLevel struct {
	V     int `json:"v"`
	Level struct {
		Name  string              `json:"name"`
		Set   json.RawMessage     `json:"set"`
		Track []store.SharedTrack `json:"track"`
	} `json:"level"`
}

// maxSharedTrack is the most club skills a shared level brings.
const maxSharedTrack = 50

// levelShareLink is a link sharing a club level and the club skills it
// tracks (own: the club's skills to track, by id). Each of its own skills
// among its rules points at the link's list of them (store.SharedItem), as
// does each it lists after them (a level from before they could be rules),
// so they arrive in order, as rules.
func levelShareLink(r *http.Request, l store.ClubLevel, own map[string]store.ClubSkill) string {
	set, err := requirements.Parse([]byte(l.Set))
	if err != nil {
		return ""
	}
	var share sharedLevel
	share.V, share.Level.Name = 1, l.Name
	at := map[string]int{} // a club skill's place in the list
	shared := func(id string) (string, bool) {
		c, ok := own[id]
		if !ok {
			return "", false
		}
		if _, seen := at[id]; !seen {
			at[id] = len(share.Level.Track)
			share.Level.Track = append(share.Level.Track, store.SharedTrack{Name: c.Name, Note: c.Note})
		}
		return store.SharedItem(at[id]), true
	}
	var rules []requirements.Rule
	for _, rule := range withClubItems(set.Rules, l.ClubItems) {
		if rule.Type == requirements.Track {
			item, ok := shared(rule.Item)
			if !ok {
				continue
			}
			rule.Item = item
		}
		rules = append(rules, rule)
	}
	set.Rules = rules
	if share.Level.Set, err = json.Marshal(set); err != nil {
		return ""
	}
	data, err := json.Marshal(share)
	if err != nil {
		return ""
	}
	return origin(r) + "/clubs/levels/shared#share=j" + base64.RawURLEncoding.EncodeToString(data)
}

// withClubItems are a level's rules with the club skills it lists after
// them (items, from before they could be rules) as rules at the end, those
// not among its rules already.
func withClubItems(rules []requirements.Rule, items []string) []requirements.Rule {
	out := slices.Clone(rules)
	for _, id := range items {
		if !slices.ContainsFunc(out, func(r requirements.Rule) bool { return r.Type == requirements.Track && r.Item == id }) {
			out = append(out, requirements.Rule{Type: requirements.Track, Item: id})
		}
	}
	return out
}

// readSharedLevel is the shared level posted (level: as its link carries it,
// decoded): its name, its requirements that can be tracked, as JSON, how many
// it had that can't, and the club skills it brings.
func readSharedLevel(raw string) (name string, set requirements.Set, left int, track []store.SharedTrack, err error) {
	var share sharedLevel
	if err := json.Unmarshal([]byte(raw), &share); err != nil || share.V != 1 {
		return "", set, 0, nil, errors.New("that isn't a shared club level")
	}
	if set, err = requirements.Parse(share.Level.Set); err != nil {
		return "", set, 0, nil, fmt.Errorf("its requirements couldn't be read: %w", err)
	}
	var rules []requirements.Rule
	for _, rule := range set.Rules {
		if trackable(rule) {
			rules = append(rules, rule)
		}
	}
	left, set.Rules = len(set.Rules)-len(rules), rules
	track = share.Level.Track
	if len(track) > maxSharedTrack {
		track = track[:maxSharedTrack]
	}
	name = share.Level.Name
	if name == "" {
		name = set.Name
	}
	return name, set, left, track, nil
}

// sharedLevelPage is where a shared level's link opens: its script reads the
// level from the link's fragment and posts it here to be shown.
func (p *competitionPages) sharedLevelPage(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.SharedLevelOpen())
}

// sharedLevelPreview shows a shared level (level: as its link carries it),
// to add to a coach's club.
func (p *competitionPages) sharedLevelPreview(w http.ResponseWriter, r *http.Request) {
	raw := r.FormValue("level")
	name, set, left, track, err := readSharedLevel(raw)
	if err != nil {
		badRequest(w, err)
		return
	}
	page := views.SharedLevelPage{Name: name, Level: raw, Left: left}
	for _, rule := range set.Rules {
		text := requirements.Describe(rule)
		// One of the club skills it brings, by its name.
		if j, err := strconv.Atoi(strings.TrimPrefix(rule.Item, "shared:")); rule.Type == requirements.Track && rule.Label == "" && err == nil && j >= 0 && j < len(track) {
			text = track[j].Name
		}
		page.Rules = append(page.Rules, text)
	}
	for _, t := range track {
		page.Track = append(page.Track, t.Name)
	}
	render(w, r, views.SharedLevel(page))
}

// importSharedLevel adds a shared level to the club of the coach whose link
// is given (link: a coach's, or a member's who coaches too, as coachFromLink
// finds them), and opens their Levels page.
func (p *competitionPages) importSharedLevel(w http.ResponseWriter, r *http.Request) {
	name, set, _, track, err := readSharedLevel(r.FormValue("level"))
	if err != nil {
		badRequest(w, err)
		return
	}
	coach, token, ok := p.coachFromLink(w, r, "/clubs/levels/import", []string{"level"})
	if !ok {
		return
	}
	data, err := json.Marshal(set)
	if err != nil {
		badRequest(w, err)
		return
	}
	_, added, err := p.st.ImportClubLevel(r.Context(), coach, name, string(data), track)
	if errors.Is(err, store.ErrLimit) {
		seeOther(w, r, coachLevelsPath(token), fmt.Sprintf("The club has %d levels, the most there's room for: remove one first.", store.MaxClubLevels))
		return
	} else if err != nil {
		failed(w, r, err)
		return
	}
	notice := "Added " + name + " to your club's levels."
	if added > 0 {
		notice += fmt.Sprintf(" It brought %s to track, now on Club skills.", plural(added, "club skill", "club skills"))
	}
	seeOther(w, r, coachLevelsPath(token), notice)
}
