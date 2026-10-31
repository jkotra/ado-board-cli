package version

import "fmt"

var (
	// Version is the current semantic version of ado-board-cli.
	Version = "0.05"

	// GitCommit is the git commit sha injected at build time (optional).
	GitCommit = ""

	// BuildDate is the RFC3339 build timestamp injected at build time (optional).
	BuildDate = ""
)

// String returns the formatted version string.
func String() string {
	if GitCommit != "" {
		return fmt.Sprintf("%s (%s)", Version, GitCommit)
	}
	return Version
}
