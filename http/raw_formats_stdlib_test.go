// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !fb_archives

package fbhttp

import (
	"errors"
	"net/http/httptest"
	"testing"
)

// The default build is what an embedder gets when it passes no tags, so it is
// the one most likely to be shipped by accident. Pin it in both directions:
// the three formats work, and the six that need the tag fail with an error
// that NAMES the tag.
func TestArchiveFormatsDefaultBuild(t *testing.T) {
	got := ArchiveFormats()
	want := []string{"zip", "tar", "targz"}
	if len(got) != len(want) {
		t.Fatalf("default build formats = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("default build formats = %v, want %v", got, want)
		}
	}
}

func TestResolveArchiverDefaultBuild(t *testing.T) {
	for _, tc := range []struct {
		algo string
		ext  string
	}{
		{"", ".zip"}, // a browser's plain "download folder" sends no algo
		{"zip", ".zip"},
		{"true", ".zip"},
		{"tar", ".tar"},
		{"targz", ".tar.gz"},
		{"true.gz", ".tar.gz"},
	} {
		r := httptest.NewRequest("GET", "/?algo="+tc.algo, nil)
		ext, w, err := resolveArchiver(r)
		if err != nil {
			t.Fatalf("algo=%q: unexpected error: %v", tc.algo, err)
		}
		if ext != tc.ext {
			t.Fatalf("algo=%q: ext = %q, want %q", tc.algo, ext, tc.ext)
		}
		if w == nil {
			t.Fatalf("algo=%q: nil writer", tc.algo)
		}
	}
}

func TestResolveArchiverDefaultBuildRejectsTaggedFormats(t *testing.T) {
	for _, algo := range []string{"tarbz2", "tarxz", "tarlz4", "tarsz", "tarbr", "tarzst"} {
		r := httptest.NewRequest("GET", "/?algo="+algo, nil)
		if _, _, err := resolveArchiver(r); !errors.Is(err, errUnsupportedAlgorithm) {
			t.Fatalf("algo=%q: err = %v, want errUnsupportedAlgorithm", algo, err)
		}
	}
}
