package fbembed

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"io/fs"
)

// distZip is the File Browser SPA (the built frontend/dist tree),
// compressed and committed so an importer gets the UI without building the
// frontend or depending on the frontend package's go:embed of dist/*.
// This keeps fbembed self-contained and keeps the importer (outpost) a
// pure-Go build with no Node toolchain.
//
// The archive is trimmed: raw .js are omitted because File Browser's
// static handler only ever serves the pre-gzipped .js.gz; non-JS .gz are
// omitted because non-JS assets are served raw. Regenerate after any
// frontend change with `go generate ./fbembed` (see gen.go).
//
//go:embed dist.zip
var distZip []byte

// distFS decodes the embedded SPA into an fs.FS rooted at the dist
// contents (so "public/index.html", "assets/...", "img/..." resolve the
// same way fs.Sub(frontend.Assets(), "dist") would). *zip.Reader
// satisfies fs.FS.
func distFS() (fs.FS, error) {
	zr, err := zip.NewReader(bytes.NewReader(distZip), int64(len(distZip)))
	if err != nil {
		return nil, fmt.Errorf("fbembed: open embedded dist.zip: %w", err)
	}
	return zr, nil
}
