// Package version holds build-time version metadata for amg. Values are
// overridden at build time via -ldflags, e.g.:
//
//	go build -ldflags "-X .../internal/version.Version=v0.1.0 -X .../internal/version.Commit=$(git rev-parse --short HEAD)"
package version

import "fmt"

var (
	// Version is the amg release version. "dev" for local/unreleased builds.
	Version = "dev"
	// Commit is the short git commit hash of the build.
	Commit = "none"
	// Date is the build timestamp in RFC3339.
	Date = "unknown"
)

// String returns a single-line human-readable version string.
func String() string {
	return fmt.Sprintf("amg %s (commit %s, built %s)", Version, Commit, Date)
}
