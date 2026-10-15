package web

import (
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// The service worker keeps pages, serves them offline, and drops a page the
// server says isn't there (404 or 410) and any kept over 30 days, run in
// node against fake caches (testdata/swtest.mjs). It skips without node.
func TestServiceWorkerKeepsAndDrops(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node to run the service worker in")
	}
	out, err := exec.Command(node, "testdata/swtest.mjs", "../static/js/sw.js").CombinedOutput()
	if err != nil {
		t.Errorf("%v\n%s", err, out)
	}
}

// The service worker keeps competition pages for offline use, and the pages
// carry the banner and render time that say when a kept copy is old.
func TestOfflineServiceWorkerAndPages(t *testing.T) {
	h := competitionServer(t)

	sw := do(t, h, http.MethodGet, "/sw.js", nil)
	for _, want := range []string{"addEventListener('fetch'", "pages-v1", "static-v1", "showNotification", "skipWaiting", "clients.claim"} {
		if !strings.Contains(sw.Body.String(), want) {
			t.Errorf("the service worker lacks %q", want)
		}
	}

	page := do(t, h, http.MethodGet, "/competitions/new", nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, `comp-offline`) || !strings.Contains(body, `data-offline-when`) {
		t.Errorf("a competition page has the offline banner: %d", page.Code)
	}
	if !regexp.MustCompile(`<meta name="page-time" content="[A-Z][a-z]+day \d\d:\d\d"`).MatchString(body) {
		t.Errorf("a competition page says when it was made: %s", body[:min(len(body), 600)])
	}
}

// The service worker marks a copy it serves by changing exactly this
// markup, so the page's banner shows: keep them in step.
func TestOfflineBannerMarkup(t *testing.T) {
	h := competitionServer(t)
	page := do(t, h, http.MethodGet, "/competitions/new", nil).Body.String()
	if !strings.Contains(page, ` comp-offline" hidden>`) {
		t.Error("the banner's markup, as sw.js expects it")
	}
	if sw := do(t, h, http.MethodGet, "/sw.js", nil).Body.String(); !strings.Contains(sw, `' comp-offline" hidden>'`) {
		t.Error("sw.js looks for the banner's markup")
	}
}
