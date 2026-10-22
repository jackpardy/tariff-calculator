// web.go
package web

import (
	"context"
	"log"
	"net/http"
	"time"

	"tariffCalculator/store"
)

// MaxHeaderBytes is the limit an http.Server should put on request headers
// (http.Server.MaxHeaderBytes); it is the same bound the handlers put on request bodies.
const MaxHeaderBytes = maxRequestBytes

// App is the whole web application: the calculator and, where competition
// storage is on, the competition pages.
type App struct {
	pages *competitionPages
}

// New builds the application over st, the competition storage. st may be nil,
// in which case the calculator works and the competition pages say storage
// isn't available.
func New(st *store.Store) *App {
	return &App{pages: newCompetitionPages(st)}
}

// Handler is every route of the application, with request bodies size-limited.
func (a *App) Handler() http.Handler {
	return routesWithPages(a.pages)
}

// Notify starts the notifier in the background, which sends the notifications
// that fall due as time passes. It returns at once, and does nothing when
// storage is off. Call it once.
func (a *App) Notify() {
	if a.pages.notify != nil {
		go a.pages.notify.run()
	}
}

// Finish waits up to wait for the emails and pushes still being sent, then
// stops sending, logging how many were left unsent. Call it once the server
// has stopped taking requests, before the store is closed.
func (a *App) Finish(wait time.Duration) {
	if a.pages.notify == nil {
		return
	}
	if left := a.pages.notify.send.stop(wait); left > 0 {
		log.Printf("Stopping: %d emails and pushes left unsent", left)
	}
}

// Sandbox makes the app the sandbox (ADR 0016): nothing emailed or pushed,
// whatever is set up, a banner on every page, the start page at /sandbox,
// and the maintainer's buttons to reset it. Its competition on the day
// restarts its day once its days are over, checked now and every hour.
// Call it once, before Handler.
func (a *App) Sandbox() {
	a.pages.makeSandbox()
	if a.pages.st != nil {
		go a.pages.rollDay()
	}
}

// ResetSandbox resets the sandbox in st, as its maintainer's button does,
// and returns how long it took: for `tariffCalculator sandbox-reset`.
func ResetSandbox(st *store.Store) (time.Duration, error) {
	p := newCompetitionPages(st)
	p.makeSandbox()
	return p.resetSandbox(context.Background())
}

// Routes is the application's handler over st alone, a fresh App's Handler
// without the notifier. It is what the demo seeds through.
func Routes(st *store.Store) http.Handler {
	return routesWith(st)
}

// DeleteExpired deletes the competitions and clubs past their keeping time,
// now and every six hours after. It never returns: run it in a goroutine.
func DeleteExpired(st *store.Store) {
	deleteExpired(st)
}
