package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/disbug-io/disbug-cli/internal/seams"
)

var callbackTokenPattern = regexp.MustCompile(`^dba_[A-Za-z0-9]{24}$`)

// CallbackResult is the token callback result returned by Listener.Wait.
type CallbackResult struct {
	Token string
	State string
	Err   error
}

// Listener serves the local auth callback endpoint for browser login.
type Listener struct {
	state     string
	success   []byte
	errorPage []byte
	listener  net.Listener
	server    *http.Server
	results   chan CallbackResult
	done      chan struct{}
	closeOnce sync.Once
	sleeper   seams.Sleeper
}

// NewListener starts a local auth callback listener.
func NewListener(
	state string,
	success,
	errorPage []byte,
	addr string,
	factory seams.ListenerFactory,
	sleeper seams.Sleeper,
) (*Listener, error) {
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	if factory == nil {
		factory = seams.DefaultListener()
	}
	if sleeper == nil {
		sleeper = seams.DefaultSleeper()
	}

	netListener, err := factory(addr)
	if err != nil {
		return nil, fmt.Errorf("bind auth listener: %w", err)
	}

	l := &Listener{
		state:     state,
		success:   success,
		errorPage: errorPage,
		listener:  netListener,
		results:   make(chan CallbackResult, 1),
		done:      make(chan struct{}),
		sleeper:   sleeper,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/cb", l.handleCallback)
	l.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := l.server.Serve(netListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			l.sendResult(CallbackResult{Err: err})
		}
	}()

	return l, nil
}

// Port returns the TCP port the listener is bound to.
func (l *Listener) Port() int {
	if l == nil || l.listener == nil {
		return 0
	}

	addr, ok := l.listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0
	}

	return addr.Port
}

// Wait waits for an auth callback result, close, context cancellation, or timeout.
func (l *Listener) Wait(ctx context.Context, timeout time.Duration) (CallbackResult, error) {
	if l == nil {
		return CallbackResult{}, errors.New("listener is nil")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	var timeoutC <-chan time.Time
	var timer *time.Timer
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		timeoutC = timer.C
		defer timer.Stop()
	}

	select {
	case result := <-l.results:
		return result, nil
	case <-l.done:
		return CallbackResult{}, http.ErrServerClosed
	case <-ctx.Done():
		return CallbackResult{}, ctx.Err()
	case <-timeoutC:
		return CallbackResult{}, context.DeadlineExceeded
	}
}

// Close shuts down the local auth callback listener.
func (l *Listener) Close() error {
	if l == nil || l.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := l.server.Shutdown(ctx)
	l.closeOnce.Do(func() {
		close(l.done)
	})

	return err
}

// BindFreePort returns an available localhost TCP port.
func BindFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("bind free port: %w", err)
	}
	defer func() {
		_ = listener.Close()
	}()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("bind free port: listener is not TCP")
	}

	return addr.Port, nil
}

// BuildAuthURL returns the Disbug agent auth URL for the local callback.
func BuildAuthURL(apiBase, callback, state, name string) string {
	base := strings.TrimRight(apiBase, "/")
	values := url.Values{}
	values.Set("callback", callback)
	values.Set("state", state)
	if name != "" {
		values.Set("name", name)
	}

	return base + "/agent-auth/?" + values.Encode()
}

// ParsePastebackURL extracts token and state from a manually pasted redirect URL.
func ParsePastebackURL(raw string) (token, state string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", fmt.Errorf("parse paste-back URL: %w", err)
	}

	values := parsed.Query()
	token = values.Get("token")
	state = values.Get("state")
	if token == "" {
		return "", "", errors.New("paste-back URL missing token")
	}
	if state == "" {
		return "", "", errors.New("paste-back URL missing state")
	}

	return token, state, nil
}

func (l *Listener) handleCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	query := r.URL.Query()
	token := query.Get("token")
	gotState := query.Get("state")

	if gotState != l.state {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(l.errorPage)
		// A state mismatch is intentionally terminal: it indicates a failed auth attempt or CSRF mismatch.
		l.sendResult(CallbackResult{Err: errors.New("state mismatch")})
		return
	}

	if !callbackTokenPattern.MatchString(token) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(l.errorPage)
		l.sendResult(CallbackResult{Err: errors.New("invalid token format")})
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(l.success)
	l.sendResult(CallbackResult{Token: token, State: gotState})
}

func (l *Listener) sendResult(result CallbackResult) {
	select {
	case l.results <- result:
	default:
	}
}
