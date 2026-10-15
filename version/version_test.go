package version

import (
	"strings"
	"testing"
)

func TestFormattedVersion(t *testing.T) {
	originalVersion := Version
	originalPrerelease := VersionPrerelease

	t.Cleanup(func() {
		Version = originalVersion
		VersionPrerelease = originalPrerelease
	})

	Version = "1.2.3"
	VersionPrerelease = "rc.1"

	if got, want := FormattedVersion(), "1.2.3-rc.1"; !strings.Contains(got, want) {
		t.Fatalf("FormattedVersion() = %q, want %q", got, want)
	}
}
