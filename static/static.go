// Package static serves the site's CSS and JavaScript from files embedded in the
// binary. Pages link to them with URL, which adds a hash of the file's content,
// so browsers can cache a versioned URL forever and still pick up a new deploy.
package static

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed css js
var files embed.FS

// Prefix is where Handler is mounted.
const Prefix = "/static/"

type asset struct {
	data []byte
	hash string // first 12 hex digits of the content's SHA-256
}

// assets maps a path relative to this directory (e.g. "js/htmx.min.js") to its file.
var assets = map[string]asset{}

func init() {
	err := fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := files.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		assets[path] = asset{data: data, hash: hex.EncodeToString(sum[:])[:12]}
		return nil
	})
	if err != nil {
		panic("static: reading embedded files: " + err.Error())
	}
}

// URL returns the versioned URL of the asset at path (e.g. "js/htmx.min.js").
// It panics for an asset that does not exist, so a bad link fails loudly in tests.
func URL(path string) string {
	a, ok := assets[path]
	if !ok {
		panic("static: no asset " + path)
	}
	return Prefix + path + "?v=" + a.hash
}

// Handler serves the assets under Prefix. A request for the current version
// (?v=hash) may be cached forever; anything else must be revalidated, which the
// ETag makes a cheap 304.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, Prefix)
		a, ok := assets[path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("ETag", `"`+a.hash+`"`)
		if r.URL.Query().Get("v") == a.hash {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeContent(w, r, path, time.Time{}, bytes.NewReader(a.data))
	})
}

// VersionHeader carries Version on every response, so a page left open
// across a deploy can tell it is out of date (static/js/version.js).
const VersionHeader = "X-App-Version"

// Version identifies this build: the first 12 hex digits of the SHA-256 of
// the running program, so any change to code, templates or assets changes it.
// If the program can't be read, the time it started stands in.
var Version = func() string {
	if exe, err := os.Executable(); err == nil {
		if data, err := os.ReadFile(exe); err == nil {
			sum := sha256.Sum256(data)
			return hex.EncodeToString(sum[:])[:12]
		}
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}()

// Bytes is the content of the asset at path (e.g. "js/sw.js"), for the few
// files served at fixed addresses.
func Bytes(path string) []byte {
	a, ok := assets[path]
	if !ok {
		panic("static: no asset " + path)
	}
	return a.data
}
