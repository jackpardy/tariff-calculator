package competitions

import "slices"

// Running orders around short rest: where a person's next flight on the same
// day starts soon after the one before (less than twice the rest wanted,
// counting to the next flight's warm-up), they go early in the first flight
// and late in the second, so their routines are further apart. It bends the
// drawn order, and the club spread, only for those people.

// restPulls says, for one flight, which entries go to the front and which to
// the back, and how short their gap is (the shortest go nearest the edge).
func (s Schedule) restPulls(people map[string][]string) map[int]map[string]int {
	pulls := map[int]map[string]int{} // flight → entry → gap; negative for the front
	if s.Setup.Rest <= 0 {
		return pulls
	}
	type turn struct {
		flight int
		entry  string
	}
	turns := map[string][]turn{}
	for i, f := range s.Flights {
		for _, id := range f.Entries {
			for _, p := range people[id] {
				turns[p] = append(turns[p], turn{i, id})
			}
		}
	}
	pull := func(flight int, entry string, gap int) {
		if pulls[flight] == nil {
			pulls[flight] = map[string]int{}
		}
		if old, ok := pulls[flight][entry]; ok && (old < 0) != (gap < 0) {
			pulls[flight][entry] = 0 // wanted at both ends: it stays where it was drawn
			return
		}
		if old, ok := pulls[flight][entry]; !ok || abs(gap) < abs(old) {
			pulls[flight][entry] = gap
		}
	}
	for _, ts := range turns {
		slices.SortFunc(ts, func(a, b turn) int {
			fa, fb := s.Flights[a.flight], s.Flights[b.flight]
			return (fa.Day*1440 + fa.Start) - (fb.Day*1440 + fb.Start)
		})
		for i := 1; i < len(ts); i++ {
			first, second := s.Flights[ts[i-1].flight], s.Flights[ts[i].flight]
			gap := second.Start - first.End
			if first.Day != second.Day || ts[i-1].flight == ts[i].flight || gap >= 2*s.Setup.Rest {
				continue
			}
			gap = max(gap, 0) + 1 // never 0, so the sign says which end
			pull(ts[i-1].flight, ts[i-1].entry, -gap)
			pull(ts[i].flight, ts[i].entry, gap)
		}
	}
	return pulls
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// orderForRest reorders one flight's running order for its pulls: the front's
// shortest gap first, the back's shortest gap last, everyone else in between
// as drawn.
func orderForRest(entries []string, pulls map[string]int) []string {
	var front, middle, back []string
	for _, id := range entries {
		switch g := pulls[id]; {
		case g < 0:
			front = append(front, id)
		case g > 0:
			back = append(back, id)
		default:
			middle = append(middle, id)
		}
	}
	slices.SortStableFunc(front, func(a, b string) int { return -pulls[a] - -pulls[b] })
	slices.SortStableFunc(back, func(a, b string) int { return pulls[b] - pulls[a] })
	return slices.Concat(front, middle, back)
}

// OrderForRest puts people with little rest between flights early in the
// first and late in the second, in every flight. people are each entry's
// people, by entry id.
func (s *Schedule) OrderForRest(people map[string][]string) {
	for i, p := range s.restPulls(people) {
		s.Flights[i].Entries = orderForRest(s.Flights[i].Entries, p)
	}
}
