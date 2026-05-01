package auth

import (
	"sync"

	"github.com/disbug-io/disbug-cli/internal/seams"
)

// BrowserOpener opens a URL in the user's browser.
type BrowserOpener = seams.BrowserOpener

var (
	openerMu sync.Mutex
	opener   BrowserOpener
)

// DefaultOpener returns the platform browser opener.
func DefaultOpener() BrowserOpener {
	return seams.DefaultBrowserOpener()
}

// Open opens rawURL with the current browser opener.
func Open(rawURL string) error {
	openerMu.Lock()
	current := opener
	openerMu.Unlock()

	if current == nil {
		current = DefaultOpener()
	}

	return current(rawURL)
}

// SwapBrowserOpener replaces the process-wide browser opener and returns the previous opener.
func SwapBrowserOpener(next BrowserOpener) BrowserOpener {
	openerMu.Lock()
	previous := opener
	opener = next
	openerMu.Unlock()

	return previous
}
