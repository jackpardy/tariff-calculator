package web

import (
	"slices"
	"testing"
)

// Every route under a club's link goes through the gate and is in
// clubRoutes, so a new one can't be left out (ADR 0015); and the table
// names no route that isn't there.
func TestEveryClubRouteHasAPermission(t *testing.T) {
	s := newTestServer(t)
	if len(s.p.clubPatterns) == 0 {
		t.Fatal("no routes went through the club gate")
	}
	var keys []string
	for _, pattern := range s.p.clubPatterns {
		key, ok := clubRoute(pattern)
		if !ok {
			t.Errorf("%s went through the club gate, but isn't under a club link", pattern)
			continue
		}
		keys = append(keys, key)
		if _, ok := clubRoutes[key]; !ok {
			t.Errorf("%s isn't in clubRoutes: say what it needs", pattern)
		}
	}
	for key := range clubRoutes {
		if !slices.Contains(keys, key) {
			t.Errorf("clubRoutes names %q, which isn't a route", key)
		}
	}
}
