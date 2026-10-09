package views

import "time"

// pageLocation is the time zone pages say when they were made in: the
// competitions are in Ireland. Falls back to UTC where the zone data is missing.
var pageLocation = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Dublin")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// pageTime is the current local time as "Saturday 14:05", put on each
// competition page so a copy kept for offline use can say when it was last
// opened (static/js/competitions.js).
func pageTime() string {
	return time.Now().In(pageLocation).Format("Monday 15:04")
}
