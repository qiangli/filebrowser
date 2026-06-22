package fbembed

import (
	"github.com/filebrowser/filebrowser/v2/auth"
	fberrors "github.com/filebrowser/filebrowser/v2/errors"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/filebrowser/filebrowser/v2/storage"
	"github.com/filebrowser/filebrowser/v2/users"
)

// newStaticStorage builds a storage.Storage with NO database. The embed is
// single-user (NoAuth) and keeps no server-side state: UI preferences live in
// the browser (localStorage), and scope/permissions/auth are recomputed from
// Options on every boot, so persistence buys nothing. The four backends below
// serve constants derived from Options and no-op every write — there is
// nothing to save and nothing to corrupt on disk.
//
// key is the JWT signing key. The host (outpost) passes a stable key so
// sessions survive a restart; when empty the caller generates an ephemeral
// one and clients re-auth transparently (NoAuth has no login prompt).
func newStaticStorage(scope string, allowWrite bool, key []byte) (*storage.Storage, error) {
	// NoAuth ignores the password, but users.Clean still requires a non-empty
	// hash. Mint one once, exactly like the old bootstrap did.
	pwd, err := users.RandomPwd(settings.DefaultMinimumPasswordLength)
	if err != nil {
		return nil, err
	}
	hashed, err := users.ValidateAndHashPwd(pwd, settings.DefaultMinimumPasswordLength)
	if err != nil {
		return nil, err
	}

	defaults := settings.UserDefaults{
		Scope:        ".",
		Locale:       "en",
		ViewMode:     users.MosaicViewMode,
		Perm:         permissions(allowWrite),
		HideDotfiles: true,
	}
	user := &users.User{ID: 1, Username: "admin", Password: hashed, LockPassword: true}
	defaults.Apply(user)

	set := &settings.Settings{
		Key:                   key,
		Signup:                false,
		HideLoginButton:       true,
		CreateUserDir:         false,
		MinimumPasswordLength: settings.DefaultMinimumPasswordLength,
		UserHomeBasePath:      settings.DefaultUsersHomeBasePath,
		Defaults:              defaults,
		AuthMethod:            auth.MethodNoAuth,
		HideDotfiles:          true,
		Tus: settings.Tus{
			ChunkSize:  settings.DefaultTusChunkSize,
			RetryCount: settings.DefaultTusRetryCount,
		},
	}
	server := &settings.Server{Root: scope, Port: "0"}

	userStore := users.NewStorage(&staticUsers{tmpl: user})
	return &storage.Storage{
		Users:    userStore,
		Settings: settings.NewStorage(&staticSettings{settings: set, server: server}),
		Auth:     auth.NewStorage(staticAuth{}, userStore),
		Share:    share.NewStorage(staticShare{}),
	}, nil
}

// staticUsers is a one-user, no-persistence users backend. Every read returns
// a fresh copy of the template (matching the DB backend, which decodes a fresh
// struct per call) so per-request handler mutation can't race the canonical
// user. Writes are no-ops: the lone user's identity/scope/perm come from
// Options, and display prefs now live in the browser.
type staticUsers struct{ tmpl *users.User }

func (s *staticUsers) GetBy(interface{}) (*users.User, error) {
	u := *s.tmpl
	u.Fs = nil // let users.Storage.Get rebuild the scoped fs for this request
	return &u, nil
}

func (s *staticUsers) Gets() ([]*users.User, error) {
	u, _ := s.GetBy(nil)
	return []*users.User{u}, nil
}

func (s *staticUsers) Save(*users.User) error                 { return nil }
func (s *staticUsers) Update(*users.User, ...string) error    { return nil }
func (s *staticUsers) DeleteByID(uint) error                  { return fberrors.ErrRootUserDeletion }
func (s *staticUsers) DeleteByUsername(string) error          { return fberrors.ErrRootUserDeletion }
func (s *staticUsers) CountAdmins() (int, error)              { return 1, nil }

// staticSettings serves the constant Settings + Server; writes no-op.
type staticSettings struct {
	settings *settings.Settings
	server   *settings.Server
}

func (s *staticSettings) Get() (*settings.Settings, error)   { return s.settings, nil }
func (s *staticSettings) Save(*settings.Settings) error      { return nil }
func (s *staticSettings) GetServer() (*settings.Server, error) { return s.server, nil }
func (s *staticSettings) SaveServer(*settings.Server) error  { return nil }

// staticAuth always resolves NoAuth; Save no-ops.
type staticAuth struct{}

func (staticAuth) Get(settings.AuthMethod) (auth.Auther, error) { return &auth.NoAuth{}, nil }
func (staticAuth) Save(auth.Auther) error                       { return nil }

// staticShare is an empty, no-op share store (sharing is disabled in the embed).
type staticShare struct{}

func (staticShare) All() ([]*share.Link, error)                    { return nil, nil }
func (staticShare) FindByUserID(uint) ([]*share.Link, error)       { return nil, nil }
func (staticShare) GetByHash(string) (*share.Link, error)          { return nil, fberrors.ErrNotExist }
func (staticShare) GetPermanent(string, uint) (*share.Link, error) { return nil, fberrors.ErrNotExist }
func (staticShare) Gets(string, uint) ([]*share.Link, error)       { return nil, nil }
func (staticShare) Save(*share.Link) error                         { return nil }
func (staticShare) Delete(string) error                            { return nil }
func (staticShare) DeleteWithPathPrefix(string, uint) error        { return nil }
