package fbembed

// Regenerate the embedded SPA after any frontend change:
//
//	go generate ./fbembed
//
// gen.sh builds frontend/dist (pnpm) and repacks the trimmed dist.zip
// embedded by assets.go. dist.zip is committed so importers build with
// only the Go toolchain (no Node).
//
//go:generate bash gen.sh
