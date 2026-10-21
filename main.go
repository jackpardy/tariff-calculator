// main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"tariffCalculator/demo"
	"tariffCalculator/store"
	"tariffCalculator/web"
)

// --- Main Function ---
func main() {
	gcDefaults()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	// Competition storage is on only where DATA_DIR is set, a directory that
	// survives deploys (ADR 0004 Decision 8). Without it, or if it can't be
	// opened, the calculator works as ever and only the competition pages
	// say storage isn't available; /healthz then fails, so a store that
	// didn't open (a migration that failed on deploy, say) is noticed.
	dir := os.Getenv("DATA_DIR")
	var st *store.Store
	if dir != "" {
		var err error
		if st, err = store.Open(context.Background(), dir); err != nil {
			log.Printf("COMPETITION STORAGE IS OFF: %v", err)
		} else {
			go web.DeleteExpired(st)
		}
	}
	// "demo" fills the storage with a made-up competition to show off, then stops.
	if len(os.Args) > 1 && os.Args[1] == "demo" {
		if st == nil {
			log.Fatal("The demo needs competition storage: set DATA_DIR.")
		}
		seed := uint64(time.Now().UnixNano())
		var err error
		if len(os.Args) > 3 && os.Args[2] == "-pages" {
			// "demo -pages FILE" also lists every person's page, for cmd/loadtest.
			var f *os.File
			if f, err = os.Create(os.Args[3]); err == nil {
				err = demo.SeedPages(web.Routes(st), os.Stdout, f, seed)
				if cerr := f.Close(); err == nil {
					err = cerr
				}
			}
		} else {
			err = demo.Seed(web.Routes(st), os.Stdout, seed)
		}
		st.Close()
		if err != nil {
			log.Fatalf("Demo: %v", err)
		}
		return
	}
	// "maintainer-link" makes the link to the maintainer's page of feedback
	// about the app, prints it and stops; a link made before stops working.
	if len(os.Args) > 1 && os.Args[1] == "maintainer-link" {
		if st == nil {
			log.Fatal("The maintainer's link needs competition storage: set DATA_DIR.")
		}
		token, err := st.NewMaintainerLink(context.Background())
		st.Close()
		if err != nil {
			log.Fatalf("Maintainer link: %v", err)
		}
		fmt.Println(strings.TrimSuffix(os.Getenv("PUBLIC_URL"), "/") + "/site/" + token)
		return
	}
	app := web.New(st)
	app.Notify()
	mux := http.NewServeMux()
	mux.Handle("/", app.Handler())
	mux.HandleFunc("GET /healthz", healthz(dir != "", st))
	// A page slowed by a crowd still arrives: at 10 seconds to write it,
	// one that took longer was cut off with no page at all.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    web.MaxHeaderBytes,
	}
	// A deploy stops the app with SIGTERM: let the requests under way finish,
	// and the emails and pushes queued, then close the database cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			log.Printf("Shutting down: %v", err)
		}
	}()
	log.Printf("Starting server on :%s\n", port)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	// Then let what's being emailed and pushed go, for a while.
	app.Finish(20 * time.Second)
	if st != nil {
		if err := st.Close(); err != nil {
			log.Printf("Closing the database: %v", err)
		}
	}
	log.Print("Stopped")
}

// The garbage collector's defaults for the app, unless GOGC or GOMEMLIMIT
// are set. What it keeps is small (about 14 MB on the demo), but a page
// can make 10 to 100 MB of garbage on the way, so at Go's default the
// collector ran about 14 times a second under load. Letting the heap grow
// to four times what's kept, but no further than about 400 MiB, cut the
// time each page took by a third or more (knowledge/operations/deploy.md).
const (
	gcPercent   = 300
	memoryLimit = 400 << 20
)

// gcDefaults sets the garbage collector's defaults for the app, where the
// environment doesn't.
func gcDefaults() {
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(gcPercent)
	}
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(memoryLimit)
	}
}

// healthz answers 200 while the app is fit to serve: always for the
// calculator alone, and with competition storage wanted (DATA_DIR set) only
// while the database is open and answers. Docker's health check and the
// deploy use it.
func healthz(wanted bool, st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if wanted {
			if st == nil {
				http.Error(w, "competition storage is off", http.StatusServiceUnavailable)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			if err := st.Ping(ctx); err != nil {
				http.Error(w, "competition storage isn't answering", http.StatusServiceUnavailable)
				return
			}
		}
		w.Write([]byte("ok\n"))
	}
}
