# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

File Browser is a **single self-contained Go binary** that serves a web file-management UI for a directory on disk. The Vue 3 frontend is built into static assets and embedded into the Go binary at compile time (`frontend/assets.go` embeds `frontend/dist/` via `go:embed`), so a release build requires building the frontend *first*, then the backend. The project is officially in **maintenance-only** mode (bug/security fixes, no new features).

## Common commands

The project uses [Task](https://taskfile.dev/) (`Taskfile.yml`), not Make.

```bash
# Full build (frontend then backend) → ./filebrowser
task build
task build:frontend     # cd frontend && pnpm install --frozen-lockfile && pnpm run build
task build:backend      # go build with version ldflags

# Backend-only dev: needs frontend/dist to exist (run task build:frontend once)
go run .                 # starts the server on :8080 by default
go build                 # plain build, no version stamping

# Backend tests
go test ./...
go test --race ./...                       # how CI runs them
go test ./files -run TestScopedFs          # single package / single test
golangci-lint run                          # config in .golangci.yml (gocritic, govet, revive)
```

Frontend (from `frontend/`, requires Node >=24 and pnpm >=10):

```bash
pnpm install
pnpm run dev         # Vite dev server — you MUST access the UI through this when developing the frontend
pnpm run build       # typecheck + vite build (writes to dist/, which the Go binary embeds)
pnpm run test        # vitest
pnpm run lint        # eslint src/
pnpm run typecheck   # vue-tsc
```

When developing the frontend you access the app through the Vite dev server, which proxies API calls to a running Go backend. When developing only the backend, a static frontend build must exist in `frontend/dist/`.

## Architecture

### Entry point and CLI

`main.go` → `cmd.Execute()`. Everything is a [Cobra](https://github.com/spf13/cobra) command rooted in `cmd/root.go`. The root command starts the HTTP server; the other `cmd/*.go` files (`users_*`, `rules_*`, `config_*`, `cmds_*`, `hash`) are **offline admin tools** that read/write the same Bolt database the server uses, for managing users, rules, settings, and command hooks without the server running. Config is layered via Viper (flags > env > config file > Bolt-stored settings).

### Request flow (backend)

1. `http.NewHandler` (`http/http.go`) builds a `gorilla/mux` router. All API routes live under `/api`; everything else falls through to the embedded SPA (`r.NotFoundHandler = index`). `server.BaseURL` is stripped via `stripPrefix`.
2. Every API handler has signature `handleFunc = func(w, r, *data) (int, error)` (`http/data.go`). The `handle()` wrapper injects a `*data` carrying the per-request `store`, `settings`, `server`, and a `runner.Runner`, and centralizes error/status handling — handlers just return `(statusCode, error)`.
3. Auth middleware `withUser` / `withAdmin` (`http/auth.go`) validates the JWT and loads the `*users.User` onto `data`, enforcing the admin bit. Authorization beyond that is the user's `Permissions` (`users/permissions.go`) plus path **Rules**.

### Key subsystems (each a top-level package)

- **`files`** — filesystem access layer. **`files.ScopedFs`** (`files/scoped.go`) is the central security boundary: it wraps `afero.BasePathFs` to confine all operations to a user's scope *and* refuses to follow symlinks whose on-disk target escapes that base. Touch this carefully — there's a dedicated symlink test suite (`http/*_symlink_test.go`). `files.FileInfo` (`files/file.go`) is the core model for listings, previews, and metadata.
- **`storage`** — backend-agnostic aggregate `Storage{ Users, Share, Auth, Settings }` (`storage/storage.go`). The only concrete backend is **BoltDB** via `asdine/storm` in `storage/bolt/`.
- **`auth`** — pluggable authentication via the `auth.Auther` interface (`auth/auth.go`). Implementations: `json` (default username/password), `proxy` (trusted reverse-proxy header), `hook` (external command), `none`. **Adding an auther requires touching three places**: the `auth/` implementation, the CLI config parser (`cmd/config.go`), and `authBackend.Get` (`storage/bolt/auth.go`).
- **`users`** — user model, bcrypt passwords, `Permissions`, and per-user `rules.Rule` lists.
- **`rules`** — allow/deny path matching (`rules.Checker` interface; plain-prefix or regex). Applied globally (settings) and per-user.
- **`runner`** — executes configured shell **command hooks** (`before_*` / `after_*` events) around file operations; gated by `EnableExec` (the `--disable-exec` flag / `disableExec` setting).
- **`settings`** — server config, defaults, branding, and tus upload config.
- **`img`** / **`diskcache`** — thumbnail/preview generation and the on-disk (or Redis) cache backing it.
- **`share`** — public share links; served under `/api/public/*` without auth.

Uploads use the **tus** resumable protocol (`/api/tus`, `http/tus_handlers.go`) with an upload cache that is either in-memory or Redis-backed (`http/upload_cache_*.go`).

### Frontend

Vue 3 + Vite + TypeScript + Pinia + vue-router + vue-i18n, in `frontend/src/`. Built output in `frontend/dist/` is embedded into the Go binary. Translations are managed externally via Transifex — don't hand-edit non-English locale files.

## Conventions

- Backend Go package is named `fbhttp` for `http/` (imported as such) to avoid clashing with stdlib `net/http`.
- Commits follow Conventional Commits (`fix:`, `chore:`, `feat:`); releases are generated from them (`.versionrc`, `commit-and-tag-version`) and the CHANGELOG is auto-maintained — don't edit `CHANGELOG.md` by hand.
- CLI documentation under `www/docs/cli` is generated by `go run . docs` (`task docs:cli:generate`); regenerate rather than editing by hand.
