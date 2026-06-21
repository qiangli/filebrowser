package fbembed

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// login hits NoAuth /api/login and returns the JWT the SPA sends as X-Auth.
func login(t *testing.T, base string) string {
	t.Helper()
	resp, err := http.Post(base+"/api/login", "application/json", nil)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(b))
}

func do(t *testing.T, method, url, token string) int {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	req.Header.Set("X-Auth", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestEmbedReadOnlyThenToggleWrite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fb.db")

	// 1. Fresh, read-only.
	h, closer, err := New(Options{Scope: dir, DBPath: dbPath, AllowWrite: false})
	if err != nil {
		t.Fatalf("New read-only: %v", err)
	}
	srv := httptest.NewServer(h)
	tok := login(t, srv.URL)

	if got := do(t, http.MethodGet, srv.URL+"/api/resources/", tok); got != http.StatusOK {
		t.Errorf("read-only GET listing = %d, want 200", got)
	}
	if got := do(t, http.MethodPost, srv.URL+"/api/resources/newdir/", tok); got != http.StatusForbidden {
		t.Errorf("read-only POST create = %d, want 403", got)
	}
	srv.Close()
	if err := closer(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// 2. Re-open the SAME store with AllowWrite — reconcile, no DB reset.
	h2, closer2, err := New(Options{Scope: dir, DBPath: dbPath, AllowWrite: true})
	if err != nil {
		t.Fatalf("New write: %v", err)
	}
	defer closer2()
	srv2 := httptest.NewServer(h2)
	defer srv2.Close()
	tok2 := login(t, srv2.URL)

	if got := do(t, http.MethodPost, srv2.URL+"/api/resources/newdir/", tok2); got != http.StatusOK {
		t.Errorf("write-enabled POST create = %d, want 200", got)
	}
	if got := do(t, http.MethodDelete, srv2.URL+"/api/resources/newdir", tok2); got != http.StatusOK && got != http.StatusNoContent {
		t.Errorf("write-enabled DELETE = %d, want 200/204", got)
	}
}

// TestEmbedPrefixContract verifies the SPA renders its base/static URLs from
// X-Forwarded-Prefix, so the handler works behind a path-prefixing proxy.
func TestEmbedPrefixContract(t *testing.T) {
	dir := t.TempDir()
	h, closer, err := New(Options{Scope: dir, DBPath: filepath.Join(dir, "fb.db")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer closer()
	srv := httptest.NewServer(h)
	defer srv.Close()

	const prefix = "/matrix/h/dragon/app/files"
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	req.Header.Set("X-Forwarded-Prefix", prefix)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"BaseURL":"`+prefix+`"`) {
		t.Errorf("index did not render BaseURL from X-Forwarded-Prefix (%q)", prefix)
	}
}
