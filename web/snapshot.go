package web

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"tariffCalculator/static"
)

// A copy of the page sent with feedback (Full): the page's HTML as it was,
// with what had been typed and opened. It's cleaned once, when it arrives,
// so the maintainer's page can show it as it looked and nothing more: no
// scripts, nothing that loads from elsewhere, and no link or form that goes
// anywhere, since every page's links are secret and the copy holds the
// sender's (taken out where it's known). It's shown with a policy that
// allows none of that either (maintainerpages.go, snapshotPolicy).

// errNotHTML is a copy that couldn't be read.
var errNotHTML = errors.New("not a page")

// cleanSnapshot reads a copy of a page as sent (gzipped or not), takes the
// sender's link token out of it, cleans it, marks where they'd scrolled to
// (#feedback-scroll) and returns it gzipped.
func cleanSnapshot(sent []byte, token string, scrollY int) ([]byte, error) {
	page := sent
	if len(sent) > 2 && sent[0] == 0x1f && sent[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(sent))
		if err != nil {
			return nil, errNotHTML
		}
		page, err = io.ReadAll(io.LimitReader(zr, maxSnapshotHTML+1))
		if err != nil || len(page) > maxSnapshotHTML {
			return nil, errNotHTML
		}
	}
	if token != "" {
		page = bytes.ReplaceAll(page, []byte(token), []byte("{token}"))
	}
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil, errNotHTML
	}
	clean(doc)
	if body := find(doc, atom.Body); body != nil {
		mark := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div, Attr: []html.Attribute{
			{Key: "id", Val: "feedback-scroll"},
			{Key: "style", Val: fmt.Sprintf("position:absolute;left:0;top:%dpx;width:1px;height:1px", max(scrollY, 0))},
		}}
		body.InsertBefore(mark, body.FirstChild)
	}
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	if err := html.Render(zw, doc); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// gone are the elements taken out of a copy: what runs, loads another page
// or changes how the copy is read.
var gone = map[atom.Atom]bool{
	atom.Script: true, atom.Noscript: true, atom.Iframe: true, atom.Frame: true, atom.Frameset: true,
	atom.Object: true, atom.Embed: true, atom.Base: true, atom.Template: true, atom.Applet: true,
	atom.Audio: true, atom.Video: true, atom.Source: true, atom.Track: true,
}

// clean takes out of a node's children what mustn't be in a copy, and
// cleans the attributes of what's left.
func clean(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode && dropped(c) {
			n.RemoveChild(c)
		} else {
			if c.Type == html.ElementNode {
				c.Attr = cleanAttrs(c)
			}
			clean(c)
		}
		c = next
	}
}

// dropped says whether an element is taken out of a copy.
func dropped(n *html.Node) bool {
	if gone[n.DataAtom] || strings.EqualFold(n.Data, "portal") {
		return true
	}
	switch n.DataAtom {
	case atom.Meta:
		return attr(n, "http-equiv") != ""
	case atom.Link:
		// Only the app's own style sheets, not the Feedback button's.
		u, ok := staticURL(attr(n, "href"))
		return !strings.EqualFold(attr(n, "rel"), "stylesheet") || !ok || strings.Contains(u, "/feedback.css")
	}
	return false
}

// cleanAttrs are an element's attributes fit for a copy: none that runs
// (on…, htmx's, Alpine's), loads or goes anywhere. A link keeps the look of
// one, going nowhere ("#"); the app's style sheets and pictures are kept,
// at their current version.
func cleanAttrs(n *html.Node) []html.Attribute {
	var out []html.Attribute
	for _, a := range n.Attr {
		key := strings.ToLower(a.Key)
		switch {
		case a.Namespace != "" && key != "href",
			strings.HasPrefix(key, "on"), strings.HasPrefix(key, "hx-"), strings.HasPrefix(key, "data-hx"),
			strings.HasPrefix(key, "x-"), strings.HasPrefix(key, "@"), strings.HasPrefix(key, ":"),
			key == "action", key == "formaction", key == "srcset", key == "srcdoc", key == "ping",
			key == "poster", key == "background", key == "manifest", key == "data", key == "codebase":
			continue
		case key == "href":
			if n.DataAtom == atom.Link {
				a.Val, _ = staticURL(a.Val)
			} else if !strings.HasPrefix(a.Val, "#") {
				a.Val = "#"
			}
		case key == "src":
			if u, ok := staticURL(a.Val); ok {
				a.Val = u
			} else if !strings.HasPrefix(a.Val, "data:image/") {
				continue
			}
		case key == "style":
			if strings.Contains(strings.ToLower(a.Val), "url(") {
				continue
			}
		case key == "target":
			continue
		}
		out = append(out, a)
	}
	return out
}

// inlineStyles puts the app's style sheets a cleaned copy links to into
// the copy itself, as it's shown: a sandboxed page's own requests for them
// can be refused (the desktop app's browser does), and a copy needs nothing
// from elsewhere this way.
func inlineStyles(page []byte) []byte {
	return styleLink.ReplaceAllFunc(page, func(link []byte) []byte {
		m := styleLink.FindSubmatch(link)
		css, ok := static.Content(string(m[1]))
		if !ok || bytes.Contains(css, []byte("</style")) {
			return nil
		}
		return append(append([]byte("<style>"), css...), "</style>"...)
	})
}

// styleLink is a link to one of the app's style sheets, as html.Render
// writes it in a cleaned copy.
var styleLink = regexp.MustCompile(`<link rel="stylesheet" href="/static/([^"?]+)(?:\?[^"]*)?"/>`)

// staticURL is an address of the app's own static files at its current
// version, not ok for anything else.
func staticURL(u string) (string, bool) {
	path, ok := strings.CutPrefix(u, static.Prefix)
	if !ok {
		return "", false
	}
	path, _, _ = strings.Cut(path, "?")
	return static.Current(path)
}

// attr is an element's attribute, "" if it hasn't it.
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

// find is the first element of a kind under n.
func find(n *html.Node, a atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == a {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := find(c, a); f != nil {
			return f
		}
	}
	return nil
}
