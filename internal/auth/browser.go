package auth

import "github.com/disbug-io/disbug-cli/internal/seams"

type BrowserOpener = seams.BrowserOpener

func DefaultOpener() BrowserOpener {
	return seams.DefaultBrowserOpener()
}
