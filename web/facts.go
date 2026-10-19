package web

import (
	"context"
	"slices"
	"sync"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// What a competition's pages work out from its entries, kept between
// requests (faster pages, for the dry run). Each page used to read every
// entry again, work out the officials rota and check every card: about 60 ms
// and 85 MB a load on the demo's 700 entries, so a burst of gymnasts opening
// their pages after a desk message could stall the server. Now the first
// page after a change works them out, and the pages share that until the
// store's next change to what they're read from (Store.Writes): a
// competition's own rows, or a person who may be in it. Check-ins, who's
// up, desk messages and the like, and other competitions' changes, keep
// them.
//
// What the cache hands out is shared between requests: read it, never
// change it. Slices of entries and cards are copies, so sorting or
// reassigning their elements is safe; the entries' own maps, slices and
// pointers, and rotaFacts' maps, are not. Tests check every page leaves the
// cache as it found it (checkFacts).

// factCache is each competition's facts, by competition id.
type factCache struct {
	mu sync.Mutex
	by map[string]*competitionFacts
}

// competitionFacts are what one competition's pages share, as read when the
// store's count of its changes stood at writes.
type competitionFacts struct {
	mu     sync.Mutex // held while they're read or worked out, so one request does it
	writes uint64
	read   bool              // false until first read, or after a failed read
	c      store.Competition // as read with them
	all    []store.Entry     // every entry (Store.Entries): withdrawn and waiting too
	rota   rotaFacts         // the live entries and the officials (liveRota)
	// The checked cards (judge), worked out when a page first asks: of the
	// entries not waiting (withdrawn too), and of the live entries.
	judgedIn, judgedLive []judged
	// The published timetable's days as the pages show them (itemsOf), with
	// the live entries and officials, worked out when a page first asks
	// (days set); nil with none published.
	days    []views.DayView
	daysSet bool
}

// factsOf are a competition's facts, read again if anything has changed
// since they were. The caller holds the facts' lock, and lets go of it when
// done with them (f.mu.Unlock).
func (p *competitionPages) factsOf(ctx context.Context, id string) (*competitionFacts, error) {
	fc := &p.facts
	fc.mu.Lock()
	if fc.by == nil {
		fc.by = map[string]*competitionFacts{}
	}
	f, ok := fc.by[id]
	if !ok {
		f = &competitionFacts{}
		fc.by[id] = f
	}
	for other, o := range fc.by {
		if other != id && o.writes != p.st.Writes(other) && o.mu.TryLock() {
			delete(fc.by, other) // out of date: free it rather than keep it
			o.mu.Unlock()
		}
	}
	fc.mu.Unlock()

	f.mu.Lock()
	if f.read && f.writes == p.st.Writes(id) {
		return f, nil
	}
	// One request reads them for everyone waiting, so it carries on if
	// its own visitor goes away.
	if err := f.readFrom(context.WithoutCancel(ctx), p, id); err != nil {
		f.read = false
		f.mu.Unlock()
		return nil, err
	}
	return f, nil
}

// readFrom reads the facts from the store afresh. The count of changes is
// taken first, so a change saved while they're read moves it past them, and
// the next request reads them again.
func (f *competitionFacts) readFrom(ctx context.Context, p *competitionPages, id string) error {
	f.writes = p.st.Writes(id)
	f.judgedIn, f.judgedLive = nil, nil
	f.days, f.daysSet = nil, false
	var err error
	if f.c, err = p.st.Competition(ctx, id); err != nil {
		return err
	}
	if f.all, err = p.st.Entries(ctx, id); err != nil {
		return err
	}
	if f.rota, err = p.readRota(ctx, f.c, f.all); err != nil {
		return err
	}
	f.read = true
	return nil
}

// entriesOf are a competition's entries, as Store.Entries has them: every
// one not removed, withdrawn and waiting too.
func (p *competitionPages) entriesOf(ctx context.Context, id string) ([]store.Entry, error) {
	f, err := p.factsOf(ctx, id)
	if err != nil {
		return nil, err
	}
	defer f.mu.Unlock()
	return slices.Clone(f.all), nil
}

// liveRota reads a competition's live entries and its officials: the one
// way the pages get them.
func (p *competitionPages) liveRota(ctx context.Context, c store.Competition) (rotaFacts, error) {
	f, err := p.factsOf(ctx, c.ID)
	if err != nil {
		return rotaFacts{}, err
	}
	defer f.mu.Unlock()
	r := f.rota
	r.entries = slices.Clone(r.entries)
	return r, nil
}

// judgedOf are a competition's cards, checked (judge): of the entries not
// waiting, withdrawn too, or of only the live ones.
func (p *competitionPages) judgedOf(ctx context.Context, id string, onlyLive bool) ([]judged, error) {
	f, err := p.factsOf(ctx, id)
	if err != nil {
		return nil, err
	}
	defer f.mu.Unlock()
	if onlyLive {
		if f.judgedLive == nil {
			f.judgedLive = judge(f.c.Competition, f.rota.entries)
		}
		return slices.Clone(f.judgedLive), nil
	}
	if f.judgedIn == nil {
		in, _ := splitWaiting(f.all)
		f.judgedIn = judge(f.c.Competition, in)
	}
	return slices.Clone(f.judgedIn), nil
}

// publishedDays are a competition's published timetable's days as the
// pages show them (itemsOf), nil with none published: each gymnast's page
// shows the whole of it.
func (p *competitionPages) publishedDays(ctx context.Context, id string) ([]views.DayView, error) {
	f, err := p.factsOf(ctx, id)
	if err != nil {
		return nil, err
	}
	defer f.mu.Unlock()
	if !f.daysSet {
		f.days, f.daysSet = daysOf(f.c, f.rota), true
	}
	return f.days, nil
}

// daysOf are a competition's published timetable's days as the pages show
// them, with its live entries and officials (publishedDays).
func daysOf(c store.Competition, r rotaFacts) []views.DayView {
	if c.Published == nil {
		return nil
	}
	return itemsOf(*c.Published, r.entries, r.officials)
}
