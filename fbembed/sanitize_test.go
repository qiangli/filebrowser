package fbembed

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captures the `which` the (rewritten) request carried to the inner handler,
// or records that the inner handler was never reached (no-op short-circuit).
type capture struct {
	reached bool
	which   []string
}

func newSanitizeFixture(cap *capture) http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.reached = true
		var env struct {
			Which []string `json:"which"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &env)
		cap.which = env.Which
		w.WriteHeader(http.StatusOK)
	})
	return sanitizeUserWrites(inner)
}

func putUsers(t *testing.T, h http.Handler, body string) *capture {
	t.Helper()
	cap := &capture{}
	h = newSanitizeFixture(cap)
	req := httptest.NewRequest(http.MethodPut, "/api/users/1", strings.NewReader(body))
	h.ServeHTTP(httptest.NewRecorder(), req)
	return cap
}

func TestSanitize_KeepsBenignField(t *testing.T) {
	cap := putUsers(t, nil, `{"what":"user","which":["viewMode"],"data":{"id":1,"viewMode":"list"}}`)
	if !cap.reached {
		t.Fatal("benign-only update should reach the inner handler")
	}
	if len(cap.which) != 1 || cap.which[0] != "ViewMode" {
		t.Fatalf("which = %v, want [ViewMode]", cap.which)
	}
}

func TestSanitize_DropsDangerousField(t *testing.T) {
	// perm is the "enable editing" write — must never reach the handler.
	cap := putUsers(t, nil, `{"what":"user","which":["perm"],"data":{"id":1,"perm":{"modify":true}}}`)
	if cap.reached {
		t.Fatalf("perm-only update must be a no-op, but inner handler saw which=%v", cap.which)
	}
}

func TestSanitize_MixedKeepsOnlyBenign(t *testing.T) {
	cap := putUsers(t, nil, `{"what":"user","which":["viewMode","perm","scope"],"data":{"id":1}}`)
	if !cap.reached {
		t.Fatal("mixed update with a benign field should reach the handler")
	}
	if len(cap.which) != 1 || cap.which[0] != "ViewMode" {
		t.Fatalf("which = %v, want only [ViewMode]", cap.which)
	}
}

func TestSanitize_AllExpandsToBenignOnly(t *testing.T) {
	// "all" would otherwise rewrite perm/scope — it must expand to the
	// benign allowlist and nothing else.
	cap := putUsers(t, nil, `{"what":"user","which":["all"],"data":{"id":1}}`)
	if !cap.reached {
		t.Fatal(`"all" update should reach the handler as a benign-only update`)
	}
	for _, f := range cap.which {
		if f == "Perm" || f == "Scope" || f == "Username" || f == "Password" ||
			f == "Commands" || f == "Rules" || f == "LockPassword" {
			t.Fatalf(`"all" expanded to a dangerous field %q: %v`, f, cap.which)
		}
	}
	if len(cap.which) != len(benignUserFields) {
		t.Fatalf("which = %v, want the %d benign fields", cap.which, len(benignUserFields))
	}
}

func TestSanitize_IgnoresNonUserPaths(t *testing.T) {
	cap := &capture{}
	h := newSanitizeFixture(cap)
	// A PUT to a different path is passed through untouched.
	req := httptest.NewRequest(http.MethodPut, "/api/resources/foo", strings.NewReader(`{"x":1}`))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !cap.reached {
		t.Fatal("non-/api/users PUT should pass straight through")
	}
}
