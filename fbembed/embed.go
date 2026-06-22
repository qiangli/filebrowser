// Package fbembed lets another Go program embed File Browser in-process:
// one call returns a ready http.Handler backed by a self-managed Bolt
// store, with no CLI, no login surface, and a read-only/download-only
// default. It is the embedding seam outpost uses to ship File Browser as
// the `files` builtin (see dhnt/docs/files-builtin-design.md), but it is
// generic — any host process can mount the returned handler under its own
// reverse proxy.
//
// The handler already speaks the cooperative-web-app prefix contract: it
// renders <base href>/StaticURL from X-Forwarded-Prefix, so the same
// instance works on loopback and behind a path-prefixing proxy with no
// per-mount config.
package fbembed

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	storm "github.com/asdine/storm/v3"

	"github.com/filebrowser/filebrowser/v2/auth"
	"github.com/filebrowser/filebrowser/v2/diskcache"
	fbhttp "github.com/filebrowser/filebrowser/v2/http"
	"github.com/filebrowser/filebrowser/v2/img"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/storage"
	"github.com/filebrowser/filebrowser/v2/storage/bolt"
	"github.com/filebrowser/filebrowser/v2/users"
)

// databasePermissions mirrors cmd's bolt file mode (owner-only).
const databasePermissions = 0640

// Options configures an embedded File Browser instance.
type Options struct {
	// Scope is the filesystem root the browser is confined to (File
	// Browser's ScopedFs, which also refuses escaping symlinks).
	// Defaults to the current working directory when empty.
	Scope string

	// AllowWrite enables every write operation together — create/upload,
	// modify/edit, rename, delete. When false (the default) the instance
	// is read-only + download-only. Command execution is never enabled
	// here. Re-calling New with a different AllowWrite reconciles the
	// existing store, so the host process can toggle write access by
	// rebuilding the handler.
	AllowWrite bool

	// DBPath is where the self-managed Bolt store lives. Created (with
	// parent dirs) on first use. Defaults to "<Scope>/.filebrowser.db"
	// when empty, but hosts should pass an explicit, app-owned path.
	DBPath string

	// ImageProcessors is the thumbnail worker count. Defaults to 4.
	ImageProcessors int
}

// New opens (creating + bootstrapping on first use) the embedded store and
// returns a File Browser http.Handler. The Bolt store is kept open for the
// lifetime of the handler (process lifetime for a builtin); the returned
// closer releases it.
//
// Auth is NoAuth — the embedding host is the access gate, exactly like
// outpost is for /shell and /ssh. The lone user is an admin (so the
// settings surface works) but Admin does NOT bypass the granular write
// perms, so AllowWrite is the real read-only⇄read-write switch.
func New(opts Options) (handler http.Handler, closer func() error, err error) {
	if opts.Scope == "" {
		if opts.Scope, err = os.Getwd(); err != nil {
			return nil, nil, fmt.Errorf("fbembed: resolve scope: %w", err)
		}
	}
	scope, err := filepath.Abs(opts.Scope)
	if err != nil {
		return nil, nil, fmt.Errorf("fbembed: abs scope: %w", err)
	}
	if opts.DBPath == "" {
		opts.DBPath = filepath.Join(scope, ".filebrowser.db")
	}
	if opts.ImageProcessors < 1 {
		opts.ImageProcessors = 4
	}

	existed := fileExists(opts.DBPath)
	if !existed {
		if dir := filepath.Dir(opts.DBPath); dir != "" {
			if err = os.MkdirAll(dir, 0700); err != nil {
				return nil, nil, fmt.Errorf("fbembed: make db dir: %w", err)
			}
		}
	}

	db, err := storm.Open(opts.DBPath, storm.BoltOptions(databasePermissions, nil))
	if err != nil {
		return nil, nil, fmt.Errorf("fbembed: open store: %w", err)
	}
	// On any error past this point, don't leak the handle.
	defer func() {
		if err != nil {
			_ = db.Close()
		}
	}()

	store, err := bolt.NewStorage(db)
	if err != nil {
		return nil, nil, fmt.Errorf("fbembed: init store: %w", err)
	}

	if existed {
		err = reconcile(store, scope, opts.AllowWrite)
	} else {
		err = bootstrap(store, scope, opts.AllowWrite)
	}
	if err != nil {
		return nil, nil, err
	}

	server, err := store.Settings.GetServer()
	if err != nil {
		return nil, nil, fmt.Errorf("fbembed: load server: %w", err)
	}

	uploadCache, err := fbhttp.NewUploadCache("")
	if err != nil {
		return nil, nil, fmt.Errorf("fbembed: upload cache: %w", err)
	}
	assets, err := assetsSource()
	if err != nil {
		return nil, nil, err
	}

	h, err := fbhttp.NewHandler(
		img.New(opts.ImageProcessors),
		diskcache.NewNoOp(),
		uploadCache,
		store,
		server,
		assets,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("fbembed: build handler: %w", err)
	}

	return sanitizeUserWrites(h), db.Close, nil
}

// benignUserFields is the allowlist of per-user settings a client may change
// through PUT/PATCH /api/users — display preferences only. Keys are the
// canonical (Title-cased) File Browser User field names the update handler
// matches on; the values are looked up case-insensitively.
var benignUserFields = []string{
	"Locale", "ViewMode", "SingleClick", "RedirectAfterCopyMove",
	"Sorting", "HideDotfiles", "DateFormat", "AceEditorTheme",
}

// sanitizeUserWrites is the embed's "host is the gate" guard on per-user
// updates. The embedding host owns the security-relevant user fields — Perm
// (read-only⇄write), Scope, plus Username/Password/LockPassword/Commands/
// Rules — via the Options + reconcile(), NOT the in-app UI. So any client
// update to /api/users is rewritten to touch ONLY the benign display
// preferences (view mode, dotfiles, theme, locale, sorting, …); a request to
// change perm/scope/etc. is silently dropped (no-op 200), never applied.
//
// This is what lets the host expose the whole File Browser through a reverse
// proxy without LAN-fencing /api/users: the view switcher, dark mode, and
// "hide dotfiles" all persist normally, while "enable editing" and scope
// changes can't be made from any client — only the host can, out of band.
func sanitizeUserWrites(next http.Handler) http.Handler {
	allowed := make(map[string]string, len(benignUserFields))
	for _, f := range benignUserFields {
		allowed[strings.ToLower(f)] = f
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method != http.MethodPut && r.Method != http.MethodPatch) ||
			!strings.HasPrefix(r.URL.Path, "/api/users/") {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		_ = r.Body.Close()
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		var env map[string]json.RawMessage
		if json.Unmarshal(body, &env) != nil {
			// Unparseable → restore the body and let File Browser 400 it.
			r.Body = io.NopCloser(bytes.NewReader(body))
			next.ServeHTTP(w, r)
			return
		}
		var which []string
		_ = json.Unmarshal(env["which"], &which)

		out := make([]string, 0, len(which))
		seen := map[string]bool{}
		addAll := func() {
			for _, f := range benignUserFields {
				if !seen[f] {
					seen[f] = true
					out = append(out, f)
				}
			}
		}
		for _, f := range which {
			lf := strings.ToLower(strings.TrimSpace(f))
			if lf == "all" {
				// "all" would update perm/scope too — expand to benign only.
				addAll()
			} else if canon, ok := allowed[lf]; ok && !seen[canon] {
				seen[canon] = true
				out = append(out, canon)
			}
			// dangerous fields (perm/scope/username/password/…) dropped.
		}
		if len(out) == 0 {
			// Nothing host-permitted to change (e.g. an "enable editing"
			// perm write) — succeed as a no-op so the client gets no error.
			w.WriteHeader(http.StatusOK)
			return
		}
		wb, _ := json.Marshal(out)
		env["which"] = wb
		nb, _ := json.Marshal(env)
		r.Body = io.NopCloser(bytes.NewReader(nb))
		r.ContentLength = int64(len(nb))
		r.Header.Set("Content-Length", strconv.Itoa(len(nb)))
		next.ServeHTTP(w, r)
	})
}

// permissions returns the lone user's perms for a given write mode. Admin
// is true so the settings/profile surface works; the granular write perms
// (create/modify/rename/delete) are what actually gate file mutation and
// are NOT bypassed by Admin. Execute and Share stay off.
func permissions(allowWrite bool) users.Permissions {
	return users.Permissions{
		Admin:    true,
		Execute:  false,
		Create:   allowWrite,
		Rename:   allowWrite,
		Modify:   allowWrite,
		Delete:   allowWrite,
		Share:    false,
		Download: true,
	}
}

// bootstrap writes the initial settings/server/auth/user for a fresh store.
func bootstrap(store *storage.Storage, scope string, allowWrite bool) error {
	key, err := settings.GenerateKey()
	if err != nil {
		return fmt.Errorf("fbembed: generate key: %w", err)
	}

	set := &settings.Settings{
		Key:                   key,
		Signup:                false,
		HideLoginButton:       true,
		CreateUserDir:         false,
		MinimumPasswordLength: settings.DefaultMinimumPasswordLength,
		UserHomeBasePath:      settings.DefaultUsersHomeBasePath,
		Defaults: settings.UserDefaults{
			Scope:    ".",
			Locale:   "en",
			ViewMode: users.MosaicViewMode,
			Perm:     permissions(allowWrite),
			// Hide dotfiles by default. The per-user toggle lives in the
			// account-settings page, which the embed hides (single-user,
			// host-is-the-gate) AND whose /api/users write is LAN-fenced by
			// the host — so this server-side default is how the preference
			// is expressed. reconcile() applies it to pre-existing stores.
			HideDotfiles: true,
		},
		AuthMethod: auth.MethodNoAuth,
		Branding:   settings.Branding{},
		Tus: settings.Tus{
			ChunkSize:  settings.DefaultTusChunkSize,
			RetryCount: settings.DefaultTusRetryCount,
		},
	}

	if err := store.Auth.Save(&auth.NoAuth{}); err != nil {
		return fmt.Errorf("fbembed: save auth: %w", err)
	}
	if err := store.Settings.Save(set); err != nil {
		return fmt.Errorf("fbembed: save settings: %w", err)
	}
	if err := store.Settings.SaveServer(&settings.Server{Root: scope, Port: "0"}); err != nil {
		return fmt.Errorf("fbembed: save server: %w", err)
	}

	// NoAuth ignores the password, but the store still wants a valid hash.
	pwd, err := users.RandomPwd(set.MinimumPasswordLength)
	if err != nil {
		return fmt.Errorf("fbembed: gen pwd: %w", err)
	}
	hashed, err := users.ValidateAndHashPwd(pwd, set.MinimumPasswordLength)
	if err != nil {
		return fmt.Errorf("fbembed: hash pwd: %w", err)
	}
	user := &users.User{Username: "admin", Password: hashed, LockPassword: true}
	set.Defaults.Apply(user)
	if err := store.Users.Save(user); err != nil {
		return fmt.Errorf("fbembed: save user: %w", err)
	}
	return nil
}

// reconcile makes an existing store match the requested scope + write mode,
// so re-calling New (e.g. on a host restart after a toggle) is idempotent
// and the DB never needs to be deleted to change settings.
func reconcile(store *storage.Storage, scope string, allowWrite bool) error {
	server, err := store.Settings.GetServer()
	if err != nil {
		return fmt.Errorf("fbembed: load server: %w", err)
	}
	if server.Root != scope {
		server.Root = scope
		if err := store.Settings.SaveServer(server); err != nil {
			return fmt.Errorf("fbembed: rebind scope: %w", err)
		}
	}

	list, err := store.Users.Gets(server.Root)
	if err != nil {
		return fmt.Errorf("fbembed: load users: %w", err)
	}
	for _, u := range list {
		var fields []string
		want := permissions(allowWrite)
		want.Admin = u.Perm.Admin // never demote/promote admin here
		if u.Perm != want {
			u.Perm = want
			fields = append(fields, "Perm")
		}
		// Apply the hide-dotfiles builtin default to pre-existing stores
		// (created before it was the default). The per-user toggle UI is
		// hidden in the embed, so this is how the preference is expressed.
		if !u.HideDotfiles {
			u.HideDotfiles = true
			fields = append(fields, "HideDotfiles")
		}
		if len(fields) == 0 {
			continue
		}
		if err := store.Users.Update(u, fields...); err != nil {
			return fmt.Errorf("fbembed: update user: %w", err)
		}
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
