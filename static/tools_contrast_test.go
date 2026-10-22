package static

import (
	"regexp"
	"strings"
	"testing"
)

// The routine tools' banner (views/tool.templ) is white text on its colour:
// readable at 4.5:1, as small text needs, the note and links included.
func TestToolBannerContrast(t *testing.T) {
	css, err := files.ReadFile("css/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(string(css), ".tool-banner {")
	if i < 0 {
		t.Fatal("no rule for .tool-banner")
	}
	rule := string(css[i:])
	rule = rule[:strings.Index(rule, "}")]
	bg := regexp.MustCompile(`background: (#[0-9a-f]{6})`).FindStringSubmatch(rule)
	fg := regexp.MustCompile(`[^-]color: (#[0-9a-f]{6})`).FindStringSubmatch(rule)
	if bg == nil || fg == nil {
		t.Fatalf("the banner's colours aren't plain: %s", rule)
	}
	if c := contrast(fg[1], bg[1]); c < 4.5 {
		t.Errorf("the banner's %s on %s is %.2f:1", fg[1], bg[1], c)
	}
}
