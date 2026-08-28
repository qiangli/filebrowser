// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build fb_archives

package fbhttp

import (
	"context"
	"io"
	"net/http"

	"github.com/mholt/archives"
)

// resolveArchiver (-tags fb_archives) writes folder downloads with
// github.com/mholt/archives, adding bz2/xz/lz4/sz/br/zst on top of the
// zip/tar/tar.gz the default build serves from the standard library.
//
// The trade is ~50 extra packages; see raw_stdlib.go for why that is opt-in.
func resolveArchiver(r *http.Request) (string, archiveWriter, error) {
	ext, archiver, err := parseQueryAlgorithm(r)
	if err != nil {
		return "", nil, err
	}
	return ext, func(ctx context.Context, w io.Writer, entries []archiveEntry) error {
		return archiver.Archive(ctx, w, toArchiveFiles(entries))
	}, nil
}

func toArchiveFiles(entries []archiveEntry) []archives.FileInfo {
	out := make([]archives.FileInfo, 0, len(entries))
	for _, e := range entries {
		out = append(out, archives.FileInfo{
			FileInfo:      e.Info,
			NameInArchive: e.NameInArchive,
			Open:          e.Open,
		})
	}
	return out
}

func parseQueryAlgorithm(r *http.Request) (string, archives.Archival, error) {
	switch r.URL.Query().Get("algo") {
	case "zip", "true", "":
		return ".zip", archives.Zip{}, nil
	case "tar":
		return ".tar", archives.Tar{}, nil
	case "targz", "true.gz":
		return ".tar.gz", archives.CompressedArchive{Compression: archives.Gz{}, Archival: archives.Tar{}}, nil
	case "tarbz2", "true.bz2":
		return ".tar.bz2", archives.CompressedArchive{Compression: archives.Bz2{}, Archival: archives.Tar{}}, nil
	case "tarxz", "true.xz":
		return ".tar.xz", archives.CompressedArchive{Compression: archives.Xz{}, Archival: archives.Tar{}}, nil
	case "tarlz4", "true.lz4":
		return ".tar.lz4", archives.CompressedArchive{Compression: archives.Lz4{}, Archival: archives.Tar{}}, nil
	case "tarsz", "true.sz":
		return ".tar.sz", archives.CompressedArchive{Compression: archives.Sz{}, Archival: archives.Tar{}}, nil
	case "tarbr", "true.br":
		return ".tar.br", archives.CompressedArchive{Compression: archives.Brotli{}, Archival: archives.Tar{}}, nil
	case "tarzst", "true.zst":
		return ".tar.zst", archives.CompressedArchive{Compression: archives.Zstd{}, Archival: archives.Tar{}}, nil
	default:
		return "", nil, errUnsupportedAlgorithm
	}
}
