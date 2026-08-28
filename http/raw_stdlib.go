// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !fb_archives

package fbhttp

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"io"
	"net/http"
)

// resolveArchiver (default build) writes folder downloads with the STANDARD
// LIBRARY: zip, tar and tar.gz cover what a browser download actually needs, and
// they cost zero extra packages.
//
// github.com/mholt/archives adds bz2/xz/lz4/sz/br/zst and brings ~50 packages
// with it (sevenzip, klauspost, dsnet, pierrec, ulikunitz, brotli, ppmd, minlz).
// That is a real capability, but it is an ARCHIVING capability rather than a
// file-browsing one, so it is opt-in here and belongs to a general-purpose
// archive tool rather than to every process that embeds a file browser.
//
// Build with `-tags fb_archives` for the full set.
func resolveArchiver(r *http.Request) (string, archiveWriter, error) {
	switch r.URL.Query().Get("algo") {
	case "zip", "true", "":
		return ".zip", writeZip, nil
	case "tar":
		return ".tar", writeTar, nil
	case "targz", "true.gz":
		return ".tar.gz", writeTarGz, nil
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

func writeTarGz(_ context.Context, w io.Writer, entries []archiveEntry) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	if err := tarEntries(tw, entries); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
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
