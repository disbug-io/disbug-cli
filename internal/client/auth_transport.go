package client

import "net/http"

type authTransport struct {
	base      http.RoundTripper
	token     string
	userAgent string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	cloned := req.Clone(req.Context())
	cloned.Header = req.Header.Clone()
	if cloned.Header == nil {
		cloned.Header = make(http.Header)
	}

	cloned.Header.Set("Authorization", "Bearer "+t.token)
	cloned.Header.Set("Accept", "application/json")
	if cloned.Header.Get("User-Agent") == "" {
		cloned.Header.Set("User-Agent", t.userAgent)
	}

	return base.RoundTrip(cloned)
}
