// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build fb_archives

package fbhttp

import (
	"net/http/httptest"
	"testing"
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
