package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestEmbeds_ArePresent(t *testing.T) {
	if len(SuccessHTML) == 0 {
		t.Fatal("SuccessHTML is empty")
	}
	if len(ErrorHTML) == 0 {
		t.Fatal("ErrorHTML is empty")
	}
}

func TestEmbeds_ContainExpectedWording(t *testing.T) {
	if !strings.Contains(strings.ToLower(string(SuccessHTML)), "signed in") {
		t.Fatal("SuccessHTML does not contain signed in wording")
	}
	if !strings.Contains(string(ErrorHTML), "Authorization failed") {
		t.Fatal("ErrorHTML does not contain Authorization failed wording")
	}
}

func TestEmbeds_NoExternalAssets(t *testing.T) {
	for name, html := range map[string][]byte{
		"success": SuccessHTML,
		"error":   ErrorHTML,
	} {
		t.Run(name, func(t *testing.T) {
			lowerHTML := strings.ToLower(string(html))
			for _, forbidden := range []string{
				`src="http`,
				`href="http`,
				`src="//`,
				`href="//`,
				"url(http",
				"<script",
				"<iframe",
				"<img",
				`<link rel="stylesheet"`,
			} {
				if strings.Contains(lowerHTML, forbidden) {
					t.Fatalf("HTML contains forbidden external asset pattern %q", forbidden)
				}
			}
		})
	}
}

func TestEmbeds_NoTokenOrStateLeaks(t *testing.T) {
	for name, html := range map[string][]byte{
		"success": SuccessHTML,
		"error":   ErrorHTML,
	} {
		t.Run(name, func(t *testing.T) {
			lowerHTML := strings.ToLower(string(html))
			for _, forbidden := range []string{
				"dba_",
				"?token=",
				"&token=",
				"?state=",
				"&state=",
			} {
				if strings.Contains(lowerHTML, forbidden) {
					t.Fatalf("HTML contains forbidden leak pattern %q", forbidden)
				}
			}
		})
	}
}

func TestEmbeds_ContainReferrerPolicy(t *testing.T) {
	for name, html := range map[string][]byte{
		"success": SuccessHTML,
		"error":   ErrorHTML,
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(strings.ToLower(string(html)), "referrer-policy") {
				t.Fatal("HTML does not contain referrer-policy")
			}
		})
	}
}

func TestListenerWithEmbeddedSuccessPageDoesNotLeakToken(t *testing.T) {
	listener, err := NewListener("S", SuccessHTML, ErrorHTML, "127.0.0.1:0", nil, nil)
	if err != nil {
		t.Fatalf("NewListener() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/cb?token=%s&state=%s", listener.Port(), url.QueryEscape(validToken), "S")
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("GET %s failed: %v", callbackURL, err)
	}
	body := readBody(t, resp)

	if got, want := resp.Header.Get("Referrer-Policy"), "no-referrer"; got != want {
		t.Fatalf("Referrer-Policy = %q, want %q", got, want)
	}
	if got := resp.Header.Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if strings.Contains(body, "dba_") {
		t.Fatalf("body leaked token prefix: %q", body)
	}

	result, err := listener.Wait(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("Wait() error = %v, want nil", err)
	}
	if result.Err != nil {
		t.Fatalf("Wait() result.Err = %v, want nil", result.Err)
	}
}
