package web

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Browser tests load pages in headless Chrome, so the JavaScript that builds a
// page's requests runs too: a page can render fine on the server and still be
// broken in a browser (the tariff sheet's hx-vals once was). They need Chrome
// or Chromium, which CI's Ubuntu runners have; without one they're skipped.

// findChrome is the path to a Chrome or Chromium to run, or "".
func findChrome() string {
	if path := os.Getenv("CHROME_PATH"); path != "" {
		return path
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "headless-shell"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	for _, path := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// browser starts headless Chrome for a test, or skips the test without one.
func browser(t *testing.T) context.Context {
	t.Helper()
	chrome := findChrome()
	if chrome == "" {
		t.Skip("no Chrome or Chromium to run browser tests (set CHROME_PATH)")
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome), chromedp.NoSandbox)
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelCtx := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	t.Cleanup(func() { cancelTimeout(); cancelCtx(); cancelAlloc() })
	return ctx
}

// sheetRoutine is the routine saved in the browser for the sheet to show: a
// tuck back (0.5) and a tuck barani (0.6).
const sheetRoutine = `{"current":"r1","routines":[{"id":"r1","name":"Sheet test","skills":[
	{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},
	{"rotation":4,"twist_distribution":[1],"takeoff_position":"Feet","shape":"Tuck"}]}]}`

// The tariff sheet page loads the routine saved in the browser and shows it:
// the page's script must post the routine for the sheet to appear at all.
func TestTariffSheetLoadsInABrowser(t *testing.T) {
	ctx := browser(t)
	srv := httptest.NewServer(routes())
	defer srv.Close()

	// Save the routine for this origin, then load the sheet with it.
	if err := chromedp.Run(ctx,
		chromedp.Navigate(srv.URL+"/tariff-sheet"),
		chromedp.Evaluate(`localStorage.setItem('trampolineRoutines', `+"`"+sheetRoutine+"`"+`)`, nil),
		chromedp.Reload(),
	); err != nil {
		t.Fatal(err)
	}
	// The sheet should appear in a moment. If it doesn't, say what the page
	// shows instead (still loading, or why it couldn't build the sheet).
	wait, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := chromedp.Run(wait, chromedp.WaitVisible(`#tariff-sheet tbody tr`, chromedp.ByQuery))
	cancel()
	if err != nil {
		var shown string
		read, cancel := context.WithTimeout(ctx, 5*time.Second)
		_ = chromedp.Run(read, chromedp.Text(`body`, &shown, chromedp.ByQuery))
		cancel()
		t.Fatalf("the tariff sheet didn't load: %v\npage: %.300s", err, strings.TrimSpace(shown))
	}

	var rows int
	var total, text string
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelectorAll('#tariff-sheet tbody tr').length`, &rows),
		chromedp.Text(`#tariff-sheet tfoot td.diff`, &total, chromedp.ByQuery),
		chromedp.Text(`#tariff-sheet`, &text, chromedp.ByQuery),
	); err != nil {
		t.Fatal(err)
	}
	if rows != 10 {
		t.Errorf("the sheet has %d rows, want a full exercise of 10", rows)
	}
	if strings.TrimSpace(total) != "1.1" {
		t.Errorf("total difficulty %q, want 1.1 (tuck back 0.5 + tuck barani 0.6)", total)
	}
	for _, want := range []string{"Tuck Back", "Tuck Barani"} {
		if !strings.Contains(text, want) {
			t.Errorf("the sheet doesn't show %s", want)
		}
	}
}
