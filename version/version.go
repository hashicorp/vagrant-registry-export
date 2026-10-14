// Package version contains build-time version information.
package version

var (
	Version           = "dev"
	VersionPrerelease = ""
	VersionMetadata   = ""
)

func FormattedVersion() string {
	formatted := Version
	if VersionPrerelease != "" {
		formatted += "-" + VersionPrerelease
	}
	if VersionMetadata != "" {
		formatted += "+" + VersionMetadata
	}
	return formatted
}
