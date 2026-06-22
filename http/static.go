package fbhttp

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/filebrowser/filebrowser/v2/auth"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/storage"
	"github.com/filebrowser/filebrowser/v2/version"
)

func handleWithStaticData(w http.ResponseWriter, r *http.Request, d *data, fSys fs.FS, file, contentType string) (int, error) {
	w.Header().Set("Content-Type", contentType)

	auther, err := d.store.Auth.Get(d.settings.AuthMethod)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	// Cooperative-app mounting (outpost / cloudbox): the public URL prefix is
	// dynamic and arrives as X-Forwarded-Prefix (e.g. "/matrix/h/<host>/app/
	// <name>"). The reverse proxy has already stripped that prefix before this
	// upstream sees the request, so request routing keeps using server.BaseURL
	// (empty for a loopback app), but every URL the SPA *emits* — assets, API
	// calls, the vue-router base, websockets, tus uploads — must carry the
	// public prefix so the browser resolves them under the proxied mount.
	// When the header is absent (direct LAN/loopback access) we fall back to
	// the statically configured server.BaseURL, so the same binary works both
	// ways with no conditional config.
	basePrefix := d.server.BaseURL
	if fwd := strings.TrimRight(r.Header.Get("X-Forwarded-Prefix"), "/"); fwd != "" {
		basePrefix = fwd
	}

	data := map[string]interface{}{
		"Name":                  d.settings.Branding.Name,
		"DisableExternal":       d.settings.Branding.DisableExternal,
		"DisableUsedPercentage": d.settings.Branding.DisableUsedPercentage,
		"Color":                 d.settings.Branding.Color,
		"BaseURL":               basePrefix,
		"Version":               version.Version,
		"StaticURL":             path.Join(basePrefix, "/static"),
		"Signup":                d.settings.Signup,
		"NoAuth":                d.settings.AuthMethod == auth.MethodNoAuth,
		"AuthMethod":            d.settings.AuthMethod,
		"LogoutPage":            d.settings.LogoutPage,
		"LoginPage":             auther.LoginPage(),
		"CSS":                   false,
		"ReCaptcha":             false,
		"Theme":                 d.settings.Branding.Theme,
		"EnableThumbs":          d.server.EnableThumbnails,
		"ResizePreview":         d.server.ResizePreview,
		"EnableExec":            d.server.EnableExec,
		"TusSettings":           d.settings.Tus,
		"HideLoginButton":       d.settings.HideLoginButton,
	}

	if d.settings.Branding.Files != "" {
		fPath := filepath.Join(d.settings.Branding.Files, "custom.css")
		_, err := os.Stat(fPath)

		if err != nil && !os.IsNotExist(err) {
			log.Printf("couldn't load custom styles: %v", err)
		}

		if err == nil {
			data["CSS"] = true
		}
	}

	if d.settings.AuthMethod == auth.MethodJSONAuth {
		raw, err := d.store.Auth.Get(d.settings.AuthMethod)
		if err != nil {
			return http.StatusInternalServerError, err
		}

		auther := raw.(*auth.JSONAuth)

		if auther.ReCaptcha != nil {
			data["ReCaptcha"] = auther.ReCaptcha.Key != "" && auther.ReCaptcha.Secret != ""
			data["ReCaptchaHost"] = auther.ReCaptcha.Host
			data["ReCaptchaKey"] = auther.ReCaptcha.Key
		}
	}

	b, err := json.Marshal(data)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	data["Json"] = template.JS(strings.ReplaceAll(string(b), `'`, `\'`))

	fileContents, err := fs.ReadFile(fSys, file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return http.StatusNotFound, err
		}
		return http.StatusInternalServerError, err
	}
	index := template.Must(template.New("index").Delims("[{[", "]}]").Parse(string(fileContents)))
	err = index.Execute(w, data)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	return 0, nil
}

func getStaticHandlers(store *storage.Storage, server *settings.Server, assetsFs fs.FS) (index, static http.Handler) {
	index = handle(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		if r.Method != http.MethodGet {
			return http.StatusNotFound, nil
		}

		w.Header().Set("x-xss-protection", "1; mode=block")
		return handleWithStaticData(w, r, d, assetsFs, "public/index.html", "text/html; charset=utf-8")
	}, "", store, server)

	static = handle(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		if r.Method != http.MethodGet {
			return http.StatusNotFound, nil
		}

		if strings.HasSuffix(r.URL.Path, "/") {
			return http.StatusNotFound, nil
		}

		const maxAge = 86400 // 1 day
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%v", maxAge))

		if d.settings.Branding.Files != "" {
			if strings.HasPrefix(r.URL.Path, "img/") {
				fPath := filepath.Join(d.settings.Branding.Files, r.URL.Path)
				_, err := os.Stat(fPath)
				if err != nil && !os.IsNotExist(err) {
					log.Printf("could not load branding file override: %v", err)
				} else if err == nil {
					http.ServeFile(w, r, fPath)
					return 0, nil
				}
			} else if r.URL.Path == "custom.css" && d.settings.Branding.Files != "" {
				http.ServeFile(w, r, filepath.Join(d.settings.Branding.Files, "custom.css"))
				return 0, nil
			}
		}

		// Non-.js assets (css, fonts, svg, images, …) are served verbatim.
		// assetsFs is a *zip.Reader, and a DEFLATE-compressed entry yields a
		// non-seekable fs.File. http.FileServer → http.ServeContent needs an
		// io.Seeker to size the body, so a compressed entry 500s with
		// "seeker can't seek" (this broke every DEFLATE-stored asset — most
		// visibly the icon/latin web fonts). Read the asset into memory and
		// serve it from a bytes.Reader, which is always seekable, so every
		// asset renders regardless of its zip compression method.
		if !strings.HasSuffix(r.URL.Path, ".js") {
			f, err := assetsFs.Open(r.URL.Path)
			if err != nil {
				return http.StatusNotFound, err
			}
			defer f.Close()

			body, err := io.ReadAll(f)
			if err != nil {
				return http.StatusInternalServerError, err
			}

			ctype := mime.TypeByExtension(path.Ext(r.URL.Path))
			if ctype == "" {
				// Go's builtin mime table omits web fonts, and a minimal
				// Windows host may have no registry entry either; set them
				// explicitly so the browser accepts the @font-face source.
				switch strings.ToLower(path.Ext(r.URL.Path)) {
				case ".woff2":
					ctype = "font/woff2"
				case ".woff":
					ctype = "font/woff"
				case ".ttf":
					ctype = "font/ttf"
				}
			}
			if ctype != "" {
				w.Header().Set("Content-Type", ctype)
			}

			var modTime time.Time
			if info, err := f.Stat(); err == nil {
				modTime = info.ModTime()
			}
			http.ServeContent(w, r, path.Base(r.URL.Path), modTime, bytes.NewReader(body))
			return 0, nil
		}

		f, err := assetsFs.Open(r.URL.Path + ".gz")
		if err != nil {
			return http.StatusNotFound, err
		}
		defer f.Close()

		acceptEncoding := r.Header.Get("Accept-Encoding")
		if strings.Contains(acceptEncoding, "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")

			if _, err := io.Copy(w, f); err != nil {
				return http.StatusInternalServerError, err
			}
		} else {
			gzReader, err := gzip.NewReader(f)
			if err != nil {
				return http.StatusInternalServerError, err
			}
			defer gzReader.Close()

			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")

			if _, err := io.Copy(w, gzReader); err != nil {
				return http.StatusInternalServerError, err
			}
		}

		return 0, nil
	}, "/static/", store, server)

	return index, static
}
