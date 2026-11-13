package web

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"tariffCalculator/store"
)

// The maintainer replaces a lost admin link from a second process (main's
// new-admin-link) while the server runs: the server counts only its own
// writes (Store.Writes), but looks every link up by its hash on each
// request, so the new link opens and the old one stops at once, even for
// pages it has cached.
func TestAdminLinkReplacedFromAnotherProcess(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	running, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { running.Close() })
	pages := newCompetitionPages(running)
	pages.notify.send.perSecond = 1e6
	t.Cleanup(func() { pages.notify.send.stop(0) })
	h := routesWithPages(pages)

	oneOff, err := store.Open(ctx, dir) // the maintainer's command
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { oneOff.Close() })

	admin := created(t, h, newCompetition())
	club, clubAdmin, err := running.CreateClub(ctx, "Cork TC")
	if err != nil {
		t.Fatal(err)
	}
	c, err := running.CompetitionByAdmin(ctx, strings.TrimPrefix(admin, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	old := []string{admin, admin + "/settings", "/clubs/admin/" + clubAdmin}
	for _, p := range old { // warm the caches
		if rec := do(t, h, http.MethodGet, p, nil); rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", p, rec.Code)
		}
	}

	newComp, err := oneOff.ReplaceCompetitionAdmin(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	newClub, err := oneOff.ReplaceClubAdmin(ctx, club.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range old {
		if rec := do(t, h, http.MethodGet, p, nil); rec.Code != http.StatusNotFound {
			t.Errorf("the old link %s still answers %d", p, rec.Code)
		}
	}
	for _, p := range []string{"/competitions/admin/" + newComp, "/competitions/admin/" + newComp + "/settings", "/clubs/admin/" + newClub} {
		if rec := do(t, h, http.MethodGet, p, nil); rec.Code != http.StatusOK {
			t.Errorf("the new link %s answers %d", p, rec.Code)
		}
	}
}
