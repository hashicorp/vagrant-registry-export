package exporter

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/vagrant-registry-export/internal/manifest"
)

func TestFilterMatches(t *testing.T) {
	tests := []struct {
		name     string
		filter   Filter
		box      string
		version  string
		provider string
		arch     string
		want     bool
	}{
		{name: "empty filter", filter: Filter{}, box: "demo", version: "1.0.0", provider: "virtualbox", arch: "amd64", want: true},
		{name: "all dimensions match", filter: Filter{Boxes: []string{"DEMO"}, Versions: []string{"1.0.0"}, Providers: []string{"VIRTUALBOX"}, Architectures: []string{"AMD64"}}, box: "demo", version: "1.0.0", provider: "virtualbox", arch: "amd64", want: true},
		{name: "box does not match", filter: Filter{Boxes: []string{"other"}}, box: "demo", want: false},
		{name: "version does not match", filter: Filter{Versions: []string{"2.0.0"}}, box: "demo", version: "1.0.0", want: false},
		{name: "provider does not match", filter: Filter{Providers: []string{"vmware"}}, box: "demo", provider: "virtualbox", want: false},
		{name: "architecture does not match", filter: Filter{Architectures: []string{"arm64"}}, box: "demo", arch: "amd64", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.matches(tt.box, tt.version, tt.provider, tt.arch); got != tt.want {
				t.Fatalf("matches() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestExportDownloadsSelectedArtifacts(t *testing.T) {
	const contents = "box contents"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, contents)
	}))
	defer server.Close()

	m := testManifest()
	var resolved, completed []string
	exp := New(Options{
		DestDir:     t.TempDir(),
		Concurrency: 1,
		Filter:      Filter{Architectures: []string{"arm64"}},
		ResolveURL: func(_, _, _, provider, architecture string) (string, string, string, error) {
			resolved = append(resolved, provider+"/"+architecture)
			return server.URL, "", "", nil
		},
		Progress: func(_, _, provider, architecture string, err error) {
			if err == nil {
				completed = append(completed, provider+"/"+architecture)
			}
		},
	})

	matched, err := exp.Export(m)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if matched != 1 {
		t.Fatalf("matched = %d, want 1", matched)
	}
	if got, want := strings.Join(resolved, ","), "virtualbox/arm64"; got != want {
		t.Fatalf("resolved artifacts = %q, want %q", got, want)
	}
	if got, want := strings.Join(completed, ","), "virtualbox/arm64"; got != want {
		t.Fatalf("completed artifacts = %q, want %q", got, want)
	}
	path := filepath.Join(exp.opts.DestDir, "example", "demo", "versions", "1.0.0", "providers", "virtualbox", "arm64", "vagrant.box")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading exported artifact: %v", err)
	}
	if string(data) != contents {
		t.Fatalf("artifact contents = %q, want %q", data, contents)
	}
}

func TestExportReturnsZeroWhenFilterMatchesNothing(t *testing.T) {
	exp := New(Options{
		DestDir: t.TempDir(),
		Filter:  Filter{Boxes: []string{"does-not-exist"}},
		ResolveURL: func(string, string, string, string, string) (string, string, string, error) {
			t.Fatal("ResolveURL should not be called when the filter matches nothing")
			return "", "", "", nil
		},
	})

	matched, err := exp.Export(testManifest())
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if matched != 0 {
		t.Fatalf("matched = %d, want 0", matched)
	}
}

func TestExportReportsResolverError(t *testing.T) {
	wantErr := errors.New("resolver unavailable")
	exp := New(Options{
		DestDir: t.TempDir(),
		ResolveURL: func(string, string, string, string, string) (string, string, string, error) {
			return "", "", "", wantErr
		},
	})
	_, err := exp.Export(testManifest())
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("Export() error = %v, want resolver error", err)
	}
}

func TestExportArtifactUsesExplicitArchitecture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "box contents")
	}))
	defer server.Close()

	exp := New(Options{
		DestDir: t.TempDir(),
		ResolveURL: func(_, _, _, _, architecture string) (string, string, string, error) {
			if architecture != "arm64" {
				t.Fatalf("resolved architecture = %q, want arm64", architecture)
			}
			return server.URL, "", "", nil
		},
	})

	provider := manifest.Provider{Name: "virtualbox", Architecture: "amd64"}
	if err := exp.exportArtifact("example", manifest.Box{
		Name: "demo",
		Tag:  "example/demo",
	}, manifest.Version{Version: "1.0.0"}, provider, "arm64"); err != nil {
		t.Fatalf("exportArtifact() error = %v", err)
	}
}

func TestExportRejectsDownloadFailureAndCleansTemporaryFile(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	exp := New(Options{
		DestDir: t.TempDir(),
		ResolveURL: func(string, string, string, string, string) (string, string, string, error) {
			return server.URL, "", "", nil
		},
	})
	if _, err := exp.Export(testManifest()); err == nil {
		t.Fatal("Export() unexpectedly succeeded")
	}
	path := exp.artifactPath(testManifest().Boxes[0], testManifest().Boxes[0].Versions[0], testManifest().Boxes[0].Versions[0].Providers[0], "amd64")
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary artifact still exists: %v", err)
	}
}

func TestVerifyChecksum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "box")
	data := []byte("box contents")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if ok, err := verifyChecksum(path, "SHA256", hex.EncodeToString(sum[:])); err != nil || !ok {
		t.Fatalf("verifyChecksum() = %t, %v", ok, err)
	}
	if ok, err := verifyChecksum(path, "sha256", "bad"); err != nil || ok {
		t.Fatalf("verifyChecksum() mismatch = %t, %v", ok, err)
	}
	if ok, err := verifyChecksum(path, "unknown", "ignored"); err != nil || !ok {
		t.Fatalf("verifyChecksum() unknown type = %t, %v", ok, err)
	}
}

func testManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Organization: "example",
		Boxes: []manifest.Box{{
			Name: "demo",
			Tag:  "example/demo",
			Versions: []manifest.Version{{
				Version: "1.0.0",
				Providers: []manifest.Provider{
					{Name: "virtualbox", Architecture: "amd64"},
					{Name: "virtualbox", Architecture: "arm64"},
				},
			}},
		}},
	}
}
