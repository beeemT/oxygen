// Package version holds version metadata injected by goreleaser.
package version

var (
	// Version is set at build time via ldflags.
	Version = "dev"
	// Commit is the git commit hash.
	Commit = ""
	// Date is the build date.
	Date = ""
)
