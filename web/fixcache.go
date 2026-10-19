package web

import (
	"crypto/sha256"
	"encoding/json"
	"sync"

	"tariffCalculator/competitions"
)

// fixCache keeps the changes that might make a timetable fit
// (competitions.Fixes), by competition id, while what they're worked out
// from stays the same. Working them out plans the timetable again for each
// change tried: most of a second, and about two on the server, which every
// load of a timetable that doesn't fit used to take.
type fixCache struct {
	mu sync.Mutex
	by map[string]*keptFixes
}

// keptFixes are one competition's fixes, worked out from what hashes to key.
type keptFixes struct {
	mu    sync.Mutex // held while they're worked out, so one request does it
	key   [sha256.Size]byte
	fixes []competitions.Fix
}

// fixesOf are competitions.Fixes for a competition's entries, events and
// timetable setup (seed 1), worked out again only when one of those
// changes. The fixes are shared: read them, never change them.
func (p *competitionPages) fixesOf(id string, entries []competitions.SchedEntry, events []string, setup competitions.Setup) []competitions.Fix {
	data, err := json.Marshal(struct {
		Entries []competitions.SchedEntry
		Events  []string
		Setup   competitions.Setup
	}{entries, events, setup})
	if err != nil {
		return competitions.Fixes(entries, events, setup, 1)
	}
	key := sha256.Sum256(data)
	fc := &p.fixes
	fc.mu.Lock()
	if fc.by == nil {
		fc.by = map[string]*keptFixes{}
	}
	k := fc.by[id]
	if k == nil {
		k = &keptFixes{}
		fc.by[id] = k
	}
	fc.mu.Unlock()
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.fixes == nil || k.key != key {
		k.key, k.fixes = key, competitions.Fixes(entries, events, setup, 1)
		if k.fixes == nil {
			k.fixes = []competitions.Fix{} // none is an answer too
		}
	}
	return k.fixes
}
