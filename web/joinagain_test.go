package web

import (
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"testing"
)

// Joining a club under a name it has asks first (someone re-tapping the join
// link in the chat), and "join anyway" makes the second member. The ask says
// nothing about the first member, and no link.
func TestJoiningUnderATakenNameAsksFirst(t *testing.T) {
	h := competitionServer(t)
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	members := club + "/members"
	join := pathIn(t, do(t, h, http.MethodGet, members, nil).Body.String(), "/clubs/join/")
	count := func() int {
		return strings.Count(do(t, h, http.MethodGet, members, nil).Body.String(), "Aoife")
	}

	// A new name joins at once, as before.
	first := redirected(t, h, join, url.Values{"name": {"Aoife Byrne"}})
	if !strings.HasPrefix(first, "/clubs/member/") {
		t.Fatalf("joined: %s", first)
	}
	firstToken := strings.TrimPrefix(withoutQuery(first), "/clubs/member/")
	was := count()

	// The same name, and the same name folded, ask and make nobody.
	for _, name := range []string{"Aoife Byrne", "  aoife   BYRNE ", "Aoífe Byrné"} {
		rec := do(t, h, http.MethodPost, join, url.Values{"name": {name}})
		body := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(body, "already has someone called Aoife Byrne") || !strings.Contains(body, "a different person with the same name") {
			t.Errorf("%q: should ask, got %d", name, rec.Code)
		}
		if !strings.Contains(body, `name="anyway" value="1"`) || !strings.Contains(body, "Join anyway") || !strings.Contains(body, "ask the competition secretary for a new link") {
			t.Errorf("%q: the ask is missing its parts", name)
		}
		if strings.Contains(body, firstToken) || strings.Contains(body, "/clubs/member/") {
			t.Errorf("%q: the ask must not show the member's link", name)
		}
		if count() != was {
			t.Errorf("%q: asking made a member", name)
		}
	}

	// A different name still joins at once.
	if got := redirected(t, h, join, url.Values{"name": {"Aoife Byrnes"}}); !strings.HasPrefix(got, "/clubs/member/") {
		t.Errorf("a different name: %s", got)
	}

	// Join anyway makes the second member, with a link of its own.
	second := redirected(t, h, join, url.Values{"name": {"Aoife Byrne"}, "anyway": {"1"}})
	if !strings.HasPrefix(second, "/clubs/member/") || withoutQuery(second) == withoutQuery(first) {
		t.Errorf("join anyway: %s", second)
	}
}

// The saved links of this browser: a member's link replaces only the same
// member's (a new link), two of one name are both kept and told apart, and the
// join page offers the pages saved at its club (static/js/competitions.js,
// run in node against a fake page: testdata/linkstest.mjs). It skips without node.
func TestSavedLinks(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node to run the saved links in")
	}
	out, err := exec.Command(node, "testdata/linkstest.mjs", "../static/js/competitions.js", "../static/js/toollinks.js").CombinedOutput()
	if err != nil {
		t.Errorf("%v\n%s", err, out)
	}
}
