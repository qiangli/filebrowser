// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build fb_archives

package fbhttp

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/s2"
	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
	"github.com/ulikunitz/xz"
)

// outpost builds with -tags fb_archives to keep the nine-format folder-download
// contract its `files` builtin shipped before the carve-out. This test is that
// contract: if a format is dropped here, outpost's download surface narrowed.
func TestArchiveFormatsTaggedBuild(t *testing.T) {
	want := []string{"zip", "tar", "targz", "tarbz2", "tarxz", "tarlz4", "tarsz", "tarbr", "tarzst"}
	got := ArchiveFormats()
	if len(got) != len(want) {
		t.Fatalf("tagged build formats = %v (%d), want %v (%d)", got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tagged build formats = %v, want %v", got, want)
		}
	}
}

// ArchiveFormats is a list; resolveArchiver is the behavior. Assert they agree,
// so the list cannot drift into a comfortable fiction.
func TestResolveArchiverAcceptsEveryAdvertisedFormat(t *testing.T) {
	for _, algo := range ArchiveFormats() {
		r := httptest.NewRequest("GET", "/?algo="+algo, nil)
		ext, w, err := resolveArchiver(r)
		if err != nil {
			t.Fatalf("algo=%q is advertised but resolveArchiver rejected it: %v", algo, err)
		}
		if ext == "" || w == nil {
			t.Fatalf("algo=%q: ext=%q writer=%v", algo, ext, w)
		}
	}
}

func TestFolderDownloadFormatCoverage(t *testing.T) {
	// 1. Dependency gate: fb_archives must exclude golang-lru/v2 (MPL).
	cmd := exec.Command("go", "list", "-deps", "-tags", "fb_archives", ".")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	depBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to list dependencies: %v: %s", err, string(depBytes))
	}
	depStr := string(depBytes)
	if strings.Contains(depStr, "golang-lru/v2") || strings.Contains(depStr, "golang-lru") {
		t.Fatalf("dependency graph includes forbidden golang-lru module:\n%s", depStr)
	}

	// 2. Folder download format coverage: all 9 formats must correctly archive entries.
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "sampledir")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	sampleFile := filepath.Join(subDir, "hello.txt")
	testData := []byte("hello format coverage test")
	if err := os.WriteFile(sampleFile, testData, 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	dirInfo, err := os.Stat(subDir)
	if err != nil {
		t.Fatalf("stat dir failed: %v", err)
	}
	fileInfo, err := os.Stat(sampleFile)
	if err != nil {
		t.Fatalf("stat file failed: %v", err)
	}

	entries := []archiveEntry{
		{
			Info:          dirInfo,
			NameInArchive: "sampledir",
			Open: func() (fs.File, error) {
				return os.Open(subDir)
			},
		},
		{
			Info:          fileInfo,
			NameInArchive: "sampledir/hello.txt",
			Open: func() (fs.File, error) {
				return os.Open(sampleFile)
			},
		},
	}

	type formatSpec struct {
		algo    string
		wantExt string
	}

	formats := []formatSpec{
		{"zip", ".zip"},
		{"tar", ".tar"},
		{"targz", ".tar.gz"},
		{"tarbz2", ".tar.bz2"},
		{"tarxz", ".tar.xz"},
		{"tarlz4", ".tar.lz4"},
		{"tarsz", ".tar.sz"},
		{"tarbr", ".tar.br"},
		{"tarzst", ".tar.zst"},
	}

	for _, tc := range formats {
		t.Run("algo="+tc.algo, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/?algo="+tc.algo, nil)
			ext, writer, err := resolveArchiver(r)
			if err != nil {
				t.Fatalf("resolveArchiver(%q) failed: %v", tc.algo, err)
			}
			if ext != tc.wantExt {
				t.Fatalf("ext = %q, want %q", ext, tc.wantExt)
			}
			if writer == nil {
				t.Fatalf("nil writer for %q", tc.algo)
			}

			var buf bytes.Buffer
			if err := writer(context.Background(), &buf, entries); err != nil {
				t.Fatalf("writer failed for %q: %v", tc.algo, err)
			}
			if buf.Len() == 0 {
				t.Fatalf("writer produced empty output for %q", tc.algo)
			}

			// Validate archive contents by extracting sampledir/hello.txt
			data := readArchiveFile(t, tc.algo, buf.Bytes(), "sampledir/hello.txt")
			if !bytes.Equal(data, testData) {
				t.Fatalf("file content mismatch for %q: got %q, want %q", tc.algo, string(data), string(testData))
			}
		})
	}
}

func readArchiveFile(t *testing.T, algo string, raw []byte, wantPath string) []byte {
	t.Helper()
	if algo == "zip" {
		zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatalf("zip.NewReader: %v", err)
		}
		for _, f := range zr.File {
			if f.Name == wantPath {
				rc, err := f.Open()
				if err != nil {
					t.Fatalf("zip open entry: %v", err)
				}
				defer rc.Close()
				b, err := io.ReadAll(rc)
				if err != nil {
					t.Fatalf("zip read entry: %v", err)
				}
				return b
			}
		}
		t.Fatalf("entry %q not found in zip archive", wantPath)
		return nil
	}

	var r io.Reader = bytes.NewReader(raw)
	switch algo {
	case "tar":
	case "targz":
		gz, err := gzip.NewReader(r)
		if err != nil {
			t.Fatalf("gzip.NewReader: %v", err)
		}
		defer gz.Close()
		r = gz
	case "tarbz2":
		bz, err := bzip2.NewReader(r, nil)
		if err != nil {
			t.Fatalf("bzip2.NewReader: %v", err)
		}
		defer bz.Close()
		r = bz
	case "tarxz":
		xzr, err := xz.NewReader(r)
		if err != nil {
			t.Fatalf("xz.NewReader: %v", err)
		}
		r = xzr
	case "tarlz4":
		r = lz4.NewReader(r)
	case "tarsz":
		r = s2.NewReader(r)
	case "tarbr":
		r = brotli.NewReader(r)
	case "tarzst":
		zr, err := zstd.NewReader(r)
		if err != nil {
			t.Fatalf("zstd.NewReader: %v", err)
		}
		defer zr.Close()
		r = zr
	default:
		t.Fatalf("unhandled format in test: %q", algo)
	}

	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next for %q: %v", algo, err)
		}
		if hdr.Name == wantPath {
			b, err := io.ReadAll(tr)
			if err != nil {
				t.Fatalf("tar read entry: %v", err)
			}
			return b
		}
	}
	t.Fatalf("entry %q not found in %s archive", wantPath, algo)
	return nil
}

