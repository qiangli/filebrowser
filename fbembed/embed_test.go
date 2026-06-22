package fbembed

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// noSeekFile embeds the fs.File interface (Stat/Read/Close only), so the
// concrete value does NOT satisfy io.Seeker even if the wrapped file does.
type noSeekFile struct{ fs.File }

// seekProbeFS overlays one non-seekable asset with a deliberately-unknown
// extension on top of the real SPA FS. That pair — non-seekable + no MIME
// type for the extension — is exactly what makes http.ServeContent's
// content sniff seek back to 0 and fail with "seeker can't seek". On
// Windows the embedded web fonts hit this (the registry has no .woff2
// type); the synthetic asset reproduces it deterministically on every OS.
type seekProbeFS struct {
	base  fs.FS
	probe string
	mem   fs.FS
}

func (o seekProbeFS) Open(name string) (fs.File, error) {
	if name == o.probe {
		f, err := o.mem.Open(name)
		if err != nil {
			return nil, err
		}
		return noSeekFile{f}, nil
	}
	return o.base.Open(name)
}

// TestStaticAssetServedWithoutSeek guards the static handler against
// requiring a seekable asset. The SPA assets live in an embedded
// *zip.Reader; on Windows a zip entry is served as a non-seekable file
// and, because the registry has no MIME type for .woff2/.woff, ServeContent
// sniffs the body and seeks back to 0 — which 500s with "seeker can't
// seek" and blanked every web font (icons + latin), wrecking the page. The
// handler now reads each non-.js asset into a bytes.Reader (always
// seekable), so the asset serves 200 even when the source file cannot seek
// and the extension is unknown. seekProbeFS reproduces that exact pair on
// any platform, so this fails without the fix off Windows too.
func TestStaticAssetServedWithoutSeek(t *testing.T) {
	const probe = "assets/zz-seekprobe.seektest" // unknown ext → forces content sniff + seek-back
	body := append([]byte("SEEKPROBE-BODY"), make([]byte, 1024)...)
	mem := fstest.MapFS{probe: &fstest.MapFile{Data: body}}

	orig := assetsSource
	assetsSource = func() (fs.FS, error) {
		base, err := orig()
		if err != nil {
			return nil, err
		}
		return seekProbeFS{base: base, probe: probe, mem: mem}, nil
	}
	defer func() { assetsSource = orig }()

	dir := t.TempDir()
	h, closer, err := New(Options{Scope: dir, AllowWrite: false})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer closer()
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/static/" + probe)
	if err != nil {
		t.Fatalf("GET %s: %v", probe, err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /static/%s = %d (%s), want 200 — a non-seekable asset with an unknown extension must still serve", probe, resp.StatusCode, strings.TrimSpace(string(got)))
	}
	if !bytes.Equal(got, body) {
		t.Errorf("served body mismatch: got %d bytes, want %d", len(got), len(body))
	}
}

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

	// 1. Fresh, read-only.
	h, closer, err := New(Options{Scope: dir, AllowWrite: false})
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

	// 2. Rebuild over the same scope with AllowWrite — perms are recomputed
	// from Options (no stored state to reconcile).
	h2, closer2, err := New(Options{Scope: dir, AllowWrite: true})
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
	h, closer, err := New(Options{Scope: dir})
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

// TestSettingsPayloadWellFormed guards GET /api/settings against null
// collection fields. The stateless settings backend must initialize the
// maps/slices the DB path used to fill in Settings.Save — a nil Commands
// map serialized as `commands: null` and crashed clients doing Object.keys
// (the File Browser Global Settings page hit exactly that).
func TestSettingsPayloadWellFormed(t *testing.T) {
	dir := t.TempDir()
	h, closer, err := New(Options{Scope: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer closer()
	srv := httptest.NewServer(h)
	defer srv.Close()

	tok := login(t, srv.URL)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/settings", nil)
	req.Header.Set("X-Auth", tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/settings: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/settings = %d, want 200", resp.StatusCode)
	}
	var payload map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	for _, field := range []string{"commands", "shell", "rules"} {
		raw, ok := payload[field]
		if !ok {
			t.Errorf("settings missing %q", field)
			continue
		}
		if string(raw) == "null" {
			t.Errorf("settings.%s is null — must be an initialized collection", field)
		}
	}
}
