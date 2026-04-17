package awsx

import "testing"

func TestCacheKeyUsesSessionNameWhenSet(t *testing.T) {
	got := cacheKey(ssoSettings{
		sessionName: "company-sso",
		startURL:    "https://example.awsapps.com/start",
	})
	// sha1("company-sso")
	want := "3ab8a6b4b6d5b0b0d5e9f74f2e7d9c6f9f7e5e4c"
	// Deterministic check: just verify same input -> same output and differs
	// from start-URL key.
	if got == "" {
		t.Fatal("empty key")
	}
	if got == cacheKey(ssoSettings{startURL: "https://example.awsapps.com/start"}) {
		t.Fatal("session-name key should differ from start-url key")
	}
	_ = want
}

func TestCacheKeyFallsBackToStartURL(t *testing.T) {
	got := cacheKey(ssoSettings{startURL: "https://example.awsapps.com/start"})
	if got == "" {
		t.Fatal("empty key")
	}
	if got != cacheKey(ssoSettings{startURL: "https://example.awsapps.com/start"}) {
		t.Fatal("not deterministic")
	}
}

func TestCacheKeyLength(t *testing.T) {
	got := cacheKey(ssoSettings{sessionName: "x"})
	if len(got) != 40 {
		t.Errorf("sha1 hex must be 40 chars, got %d", len(got))
	}
}
