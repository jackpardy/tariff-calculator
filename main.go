// main.go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
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
	// say storage isn't available.
	var st *store.Store
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		var err error
		if st, err = store.Open(context.Background(), dir); err != nil {
			log.Printf("Competition storage is off: %v", err)
		} else {
			defer st.Close()
			go web.DeleteExpired(st)
		}
	}
	// "demo" fills the storage with a made-up competition to show off, then stops.
	if len(os.Args) > 1 && os.Args[1] == "demo" {
		if st == nil {
			log.Fatal("The demo needs competition storage: set DATA_DIR.")
		}
		if err := demo.Seed(web.Routes(st), os.Stdout, uint64(time.Now().UnixNano())); err != nil {
			log.Fatalf("Demo: %v", err)
		}
		return
	}
	app := web.New(st)
	app.Notify()
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    web.MaxHeaderBytes,
	}
	log.Printf("Starting server on :%s\n", port)
	log.Fatal(srv.ListenAndServe())
}
