// main.go
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tariffCalculator/demo"
	"tariffCalculator/store"
	"tariffCalculator/web"
)

// --- Main Function ---
func main() {
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
	app := web.New(st)
	app.Notify()
	mux := http.NewServeMux()
	mux.Handle("/", app.Handler())
	mux.HandleFunc("GET /healthz", healthz(dir != "", st))
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    web.MaxHeaderBytes,
	}
	// A deploy stops the app with SIGTERM: let the requests under way finish,
	// then close the database cleanly.
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
	if st != nil {
		if err := st.Close(); err != nil {
			log.Printf("Closing the database: %v", err)
		}
	}
	log.Print("Stopped")
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
