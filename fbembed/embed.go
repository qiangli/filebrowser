// Package fbembed lets another Go program embed File Browser in-process:
// one call returns a ready http.Handler with no CLI, no login surface, and a
// read-only/download-only default. It is the embedding seam outpost uses to
// ship File Browser as the `files` builtin (see dhnt/docs/files-builtin-design.md),
// but it is generic — any host process can mount the returned handler under
// its own reverse proxy.
//
// The embed is single-user (NoAuth) and STATELESS: it keeps no database and no
// on-disk state. Scope, permissions and auth are recomputed from Options on
// every boot, and per-user UI preferences live in the browser (localStorage),
// so there is nothing to persist. The embedding host is the access gate,
// exactly like outpost is for /shell and /ssh.
//
// The handler already speaks the cooperative-web-app prefix contract: it
// renders <base href>/StaticURL from X-Forwarded-Prefix, so the same instance
// works on loopback and behind a path-prefixing proxy with no per-mount config.
package fbembed

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/filebrowser/filebrowser/v2/diskcache"
	fbhttp "github.com/filebrowser/filebrowser/v2/http"
	"github.com/filebrowser/filebrowser/v2/img"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
)

// Options configures an embedded File Browser instance.
type Options struct {
	// Scope is the filesystem root the browser is confined to (File
	// Browser's ScopedFs, which also refuses escaping symlinks).
	// Defaults to the current working directory when empty.
	Scope string

	// AllowWrite enables every write operation together — create/upload,
	// modify/edit, rename, delete. When false (the default) the instance
	// is read-only + download-only. Command execution is never enabled
	// here. Rebuilding the handler with a different AllowWrite simply
	// recomputes the lone user's perms — there is no stored state to
	// reconcile.
	AllowWrite bool

	// SigningKey is the JWT signing key. When non-empty (the host persists
	// one out of band, e.g. outpost in its agent.json) sessions survive a
	// process restart. When empty a fresh ephemeral key is generated each
	// boot; clients re-auth transparently since NoAuth has no login prompt.
	SigningKey []byte

	// ImageProcessors is the thumbnail worker count. Defaults to 4.
	ImageProcessors int
}

// New returns a stateless File Browser http.Handler. The returned closer is a
// no-op (there is no store to release); it is retained so callers don't have
// to change shape.
//
// Auth is NoAuth — the embedding host is the access gate. The lone user is an
// admin (so the settings surface works) but Admin does NOT bypass the granular
// write perms, so AllowWrite is the real read-only⇄read-write switch.
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
	if opts.ImageProcessors < 1 {
		opts.ImageProcessors = 4
	}

	key := opts.SigningKey
	if len(key) == 0 {
		if key, err = settings.GenerateKey(); err != nil {
			return nil, nil, fmt.Errorf("fbembed: generate key: %w", err)
		}
	}

	store, err := newStaticStorage(scope, opts.AllowWrite, key)
	if err != nil {
		return nil, nil, fmt.Errorf("fbembed: init store: %w", err)
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

	return h, func() error { return nil }, nil
}

// permissions returns the lone user's perms for a given write mode. Admin is
// true so the settings/profile surface works; the granular write perms
// (create/modify/rename/delete) are what actually gate file mutation and are
// NOT bypassed by Admin. Execute and Share stay off.
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
