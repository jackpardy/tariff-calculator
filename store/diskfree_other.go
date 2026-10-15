//go:build !unix

package store

import "errors"

// diskFree can't tell the space free here, so certificates aren't refused
// for it.
func diskFree(string) (uint64, error) {
	return 0, errors.New("free space unknown on this system")
}
