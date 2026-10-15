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
