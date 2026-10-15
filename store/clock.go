package store

import "time"

// SetClock makes the store read the time from now instead of the system
// clock, so tests can run a competition day at a fixed time. Times are
// still stored in UTC.
func (s *Store) SetClock(now func() time.Time) {
	s.now = func() time.Time { return now().UTC() }
}
