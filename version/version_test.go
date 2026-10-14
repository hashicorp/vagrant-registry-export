package version

import "testing"

func TestFormattedVersion(t *testing.T) {
	originalVersion := Version
	originalPrerelease := VersionPrerelease
	originalMetadata := VersionMetadata
	t.Cleanup(func() {
		Version = originalVersion
		VersionPrerelease = originalPrerelease
		VersionMetadata = originalMetadata
	})

	Version = "1.2.3"
	VersionPrerelease = "rc.1"
	VersionMetadata = "build.42"

	if got, want := FormattedVersion(), "1.2.3-rc.1+build.42"; got != want {
		t.Fatalf("FormattedVersion() = %q, want %q", got, want)
	}
}
