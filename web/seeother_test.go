package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"tariffCalculator/views"
)

// A redirect after a form carries its notice first in the query, before
// any #part; with nothing to say, the page as it is.
func TestSeeOther(t *testing.T) {
	for _, c := range []struct{ to, notice, want string }{
		{"/a", "Saved.", "/a?notice=Saved."},
		{"/a", "", "/a"},
		{"/a?discipline=tumbling", "Saved, again.", "/a?notice=Saved%2C+again.&discipline=tumbling"},
		{"/a#ties", "Tied.", "/a?notice=Tied.#ties"},
		{"/a?x=1#ties", "Tied.", "/a?notice=Tied.&x=1#ties"},
	} {
		rec := httptest.NewRecorder()
		seeOther(rec, httptest.NewRequest(http.MethodPost, "/", nil), c.to, c.notice)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != c.want {
			t.Errorf("%s with %q: %d %s, want %s", c.to, c.notice, rec.Code, rec.Header().Get("Location"), c.want)
		}
	}
	if got := views.Count(1, "entry", "entries") + ", " + views.Count(0, "entry", "entries") + ", " + views.Count(12, "entry", "entries"); got != "1 entry, 0 entries, 12 entries" {
		t.Errorf("counted: %s", got)
	}
}
