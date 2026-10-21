// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build fb_archives

package fbhttp

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"io"
	"net/http"

	"github.com/andybalholm/brotli"
	"github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/s2"
	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
	"github.com/ulikunitz/xz"
)

// resolveArchiver (-tags fb_archives) writes folder downloads with permissive
// archive packages, adding bz2/xz/lz4/sz/br/zst on top of the zip/tar/tar.gz
// the default build serves from the standard library.
func resolveArchiver(r *http.Request) (string, archiveWriter, error) {
	switch r.URL.Query().Get("algo") {
	case "zip", "true", "":
		return ".zip", writeZip, nil
	case "tar":
		return ".tar", writeTar, nil
	case "targz", "true.gz":
		return ".tar.gz", writeTarGz, nil
	case "tarbz2", "true.bz2":
		return ".tar.bz2", writeTarBz2, nil
	case "tarxz", "true.xz":
		return ".tar.xz", writeTarXz, nil
	case "tarlz4", "true.lz4":
		return ".tar.lz4", writeTarLz4, nil
	case "tarsz", "true.sz":
		return ".tar.sz", writeTarSz, nil
	case "tarbr", "true.br":
		return ".tar.br", writeTarBr, nil
	case "tarzst", "true.zst":
		return ".tar.zst", writeTarZst, nil
	default:
		return "", nil, errUnsupportedAlgorithm
	}
}

func writeZip(_ context.Context, w io.Writer, entries []archiveEntry) error {
	zw := zip.NewWriter(w)
	for _, e := range entries {
		hdr, err := zip.FileInfoHeader(e.Info)
		if err != nil {
			return err
		}
		hdr.Name = e.NameInArchive
		hdr.Method = zip.Deflate
		if e.Info.IsDir() {
			hdr.Name += "/"
		}
		dst, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		if e.Info.IsDir() {
			continue
		}
		if err := copyEntry(dst, e); err != nil {
			return err
		}
	}
	return zw.Close()
}

func writeTar(_ context.Context, w io.Writer, entries []archiveEntry) error {
	tw := tar.NewWriter(w)
	if err := tarEntries(tw, entries); err != nil {
		return err
	}
	return tw.Close()
}

func writeCompressedTar(cw io.WriteCloser, entries []archiveEntry) error {
	tw := tar.NewWriter(cw)
	if err := tarEntries(tw, entries); err != nil {
		_ = tw.Close()
		_ = cw.Close()
		return err
	}
	if err := tw.Close(); err != nil {
		_ = cw.Close()
		return err
	}
	return cw.Close()
}

func writeTarGz(_ context.Context, w io.Writer, entries []archiveEntry) error {
	return writeCompressedTar(gzip.NewWriter(w), entries)
}

func writeTarBz2(_ context.Context, w io.Writer, entries []archiveEntry) error {
	bw, err := bzip2.NewWriter(w, nil)
	if err != nil {
		return err
	}
	return writeCompressedTar(bw, entries)
}

func writeTarXz(_ context.Context, w io.Writer, entries []archiveEntry) error {
	xw, err := xz.NewWriter(w)
	if err != nil {
		return err
	}
	return writeCompressedTar(xw, entries)
}

func writeTarLz4(_ context.Context, w io.Writer, entries []archiveEntry) error {
	return writeCompressedTar(lz4.NewWriter(w), entries)
}

func writeTarSz(_ context.Context, w io.Writer, entries []archiveEntry) error {
	return writeCompressedTar(s2.NewWriter(w), entries)
}

func writeTarBr(_ context.Context, w io.Writer, entries []archiveEntry) error {
	return writeCompressedTar(brotli.NewWriter(w), entries)
}

func writeTarZst(_ context.Context, w io.Writer, entries []archiveEntry) error {
	zw, err := zstd.NewWriter(w)
	if err != nil {
		return err
	}
	return writeCompressedTar(zw, entries)
}

func tarEntries(tw *tar.Writer, entries []archiveEntry) error {
	for _, e := range entries {
		hdr, err := tar.FileInfoHeader(e.Info, "")
		if err != nil {
			return err
		}
		// FileInfoHeader only sees the base name; the archive path is ours.
		hdr.Name = e.NameInArchive
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if e.Info.IsDir() {
			continue
		}
		if err := copyEntry(tw, e); err != nil {
			return err
		}
	}
	return nil
}

// copyEntry opens one member and streams it, closing it before returning so a
// large tree does not hold every file descriptor open at once.
func copyEntry(dst io.Writer, e archiveEntry) error {
	src, err := e.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	_, err = io.Copy(dst, src)
	return err
}

// ArchiveFormats reports the `algo` values this build accepts for a folder
// download, in a stable order. See the default build's copy in raw_stdlib.go
// for why this is exported.
//
// -tags fb_archives: the full set.
func ArchiveFormats() []string {
	return []string{"zip", "tar", "targz", "tarbz2", "tarxz", "tarlz4", "tarsz", "tarbr", "tarzst"}
}
