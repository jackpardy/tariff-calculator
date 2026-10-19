package static

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// luminance is a #rrggbb colour's relative luminance (WCAG 2).
func luminance(hex string) float64 {
	var c [3]float64
	for i := range c {
		v, _ := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		x := float64(v) / 255
		if x <= 0.03928 {
			c[i] = x / 12.92
		} else {
			c[i] = math.Pow((x+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]
}

func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

// The primary and success buttons, and their colours as text, are readable:
// at least 4.5:1 for white on them and for them on white.
func TestPrimaryAndSuccessContrast(t *testing.T) {
	css, err := files.ReadFile("css/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, sel := range []string{
		`.button.is-primary:not(.is-light):not(.is-outlined), .tag.is-primary`,
		`.button.is-primary:not(.is-light):not(.is-outlined):hover`,
		`.button.is-success:not(.is-light):not(.is-outlined), .tag.is-success`,
		`.button.is-success:not(.is-light):not(.is-outlined):hover`,
		`.has-text-primary`,
		`.has-text-success`,
	} {
		i := strings.Index(string(css), sel)
		if i < 0 {
			t.Errorf("no rule for %s", sel)
			continue
		}
		rule := string(css[i:])
		rule = rule[:strings.Index(rule, "}")]
		m := regexp.MustCompile(`(?:background-)?color: (#[0-9a-f]{6})`).FindStringSubmatch(rule)
		if m == nil {
			t.Errorf("%s has no colour", sel)
			continue
		}
		if c := contrast(m[1], "#ffffff"); c < 4.5 {
			t.Errorf("%s: %s and white are %.2f:1", sel, m[1], c)
		}
	}
}

// The palette's state and role colours (views/status.templ) read at 4.5:1 or
// better: each state's text on its background and on white, each role's
// colour on white.
func TestPaletteContrast(t *testing.T) {
	css, err := files.ReadFile("css/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	token := func(name string) string {
		m := regexp.MustCompile(`--` + regexp.QuoteMeta(name) + `: (#[0-9a-f]{6});`).FindStringSubmatch(string(css))
		if m == nil {
			t.Fatalf("no colour --%s", name)
		}
		return m[1]
	}
	for _, s := range []string{"not-yet", "in-progress", "done", "attention", "stopped"} {
		ink, bg := token("state-"+s+"-ink"), token("state-"+s+"-bg")
		if c := contrast(ink, bg); c < 4.5 {
			t.Errorf("%s: %s on %s is %.2f:1", s, ink, bg, c)
		}
		if c := contrast(ink, "#ffffff"); c < 4.5 {
			t.Errorf("%s: %s on white is %.2f:1", s, ink, c)
		}
	}
	for _, r := range []string{"gymnast", "coach", "club", "organiser"} {
		if c := contrast(token("role-"+r), "#ffffff"); c < 4.5 {
			t.Errorf("%s: %s on white is %.2f:1", r, token("role-"+r), c)
		}
	}
}

// Text on the competition pages reads at 4.5:1 or better on white: grey
// small text, red Remove and Delete, the outlined danger button, white on
// the danger button, and the shared look's own greys.
func TestTextContrast(t *testing.T) {
	css, err := files.ReadFile("css/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, sel := range []string{
		`.has-text-grey {`,
		`.has-text-danger {`,
		`.button.is-danger:not(.is-light):not(.is-outlined) {`,
		`.button.is-danger.is-outlined {`,
		`.comp-opens-detail {`,
		`.comp-empty {`,
		`.comp-key {`,
		`.comp-scratched {`,
	} {
		for rest, found := string(css), false; ; {
			i := strings.Index(rest, sel)
			if i < 0 {
				if !found {
					t.Errorf("no rule for %s", sel)
				}
				break
			}
			found = true
			rule := rest[i:]
			rule = rule[:strings.Index(rule, "}")]
			rest = rest[i+len(sel):]
			for _, m := range regexp.MustCompile(`(?:^|[^-])(?:background-)?color: (#[0-9a-f]{6})`).FindAllStringSubmatch(rule, -1) {
				if c := contrast(m[1], "#ffffff"); c < 4.5 {
					t.Errorf("%s: %s and white are %.2f:1", sel, m[1], c)
				}
			}
		}
	}
	// A chip is never smaller than 12px, even inside small text.
	if !strings.Contains(string(css), ".chip { display: inline-flex; align-items: baseline; gap: 0.3em; max-width: 100%; font-size: max(0.8em, 0.75rem);") {
		t.Error("a chip's text has a floor of 0.75rem")
	}
}
