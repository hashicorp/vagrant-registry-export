// Package version contains build-time version information.
package version

import "time"

var (
	BuildDate string
	GitCommit string

	Version           = "0.1.1"
	VersionPrerelease = "pre.0"
)

func FormattedVersion() string {
	formatted := Version
	if VersionPrerelease != "" {
		formatted += "-" + VersionPrerelease
	}

	built, _ := time.Parse(time.RFC3339, BuildDate)
	if !built.IsZero() {
		formatted += "\nBuildDate " + built.Format(time.RFC3339)
	}
	if GitCommit != "" {
		formatted += "\nRevision " + GitCommit
	}

	return formatted
}
