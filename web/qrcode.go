// qrcode.go: QR codes for share links, drawn as SVG.
package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"rsc.io/qr"
)

// quietZone is the blank border a QR code needs to be read, in modules.
const quietZone = 4

// handleQR draws a QR code (SVG) for the text posted in text, e.g. a share
// link, so it can be scanned from another phone.
func handleQR(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	text := r.FormValue("text")
	if text == "" {
		badRequest(w, errors.New("nothing to put in a QR code"))
		return
	}
	// Medium error correction reads more reliably; long links fall back to low.
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		code, err = qr.Encode(text, qr.L)
	}
	if err != nil {
		badRequest(w, errors.New("that's too much for one QR code; share fewer routines at a time"))
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, qrSVG(code))
}

// qrSVG draws a code's dark modules as one path, a run of modules per row.
func qrSVG(code *qr.Code) string {
	size := code.Size + 2*quietZone
	var path strings.Builder
	for y := range code.Size {
		for x := 0; x < code.Size; {
			if !code.Black(x, y) {
				x++
				continue
			}
			run := 1
			for x+run < code.Size && code.Black(x+run, y) {
				run++
			}
			fmt.Fprintf(&path, "M%d %dh%dv1h-%dz", x+quietZone, y+quietZone, run, run)
			x += run
		}
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges" role="img" aria-label="QR code"><rect width="%d" height="%d" fill="#fff"/><path d="%s" fill="#000"/></svg>`,
		size, size, size, size, path.String())
}
