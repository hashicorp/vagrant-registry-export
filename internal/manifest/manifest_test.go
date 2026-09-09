package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/vagrant-registry-export/internal/hcp"
)

func TestFromRegistry(t *testing.T) {
	registry := &hcp.Registry{
		Name: "example",
		Boxes: []*hcp.Box{
			{
				Name:        "demo",
				IsPrivate:   true,
				Description: "demo box",
				Versions: []*hcp.Version{
					{
						Name:        "1.0.0",
						Description: "first release",
						Providers: []*hcp.Provider{
							{
								Name: "virtualbox",
								Architectures: []*hcp.Architecture{
									{ArchitectureType: "amd64"},
									nil,
								},
							},
							nil,
						},
					},
					nil,
				},
			},
			nil,
		},
	}

	got := FromRegistry(registry)
	if got.Organization != "example" || len(got.Boxes) != 1 {
		t.Fatalf("manifest identity = %#v", got)
	}
	box := got.Boxes[0]
	if box.Tag != "example/demo" || !box.Private || len(box.Versions) != 1 {
		t.Fatalf("box conversion = %#v", box)
	}
	if len(box.Versions[0].Providers) != 1 {
		t.Fatalf("provider conversion = %#v", box.Versions[0].Providers)
	}
	provider := box.Versions[0].Providers[0]
	if provider.Name != "virtualbox" || provider.Architecture != "amd64" || provider.DownloadURL != "" {
		t.Fatalf("provider conversion = %#v", provider)
	}
}

func TestManifestMarshalWriteLoad(t *testing.T) {
	want := &Manifest{
		Organization: "example",
		Boxes: []Box{{
			Name: "demo",
			Versions: []Version{{
				Version:   "1.0.0",
				Providers: []Provider{{Name: "virtualbox", Architecture: "amd64"}},
			}},
		}},
	}

	data, err := Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Marshal() produced invalid JSON: %v", err)
	}
	if decoded.Organization != want.Organization || len(decoded.Boxes) != 1 {
		t.Fatalf("decoded manifest = %#v", decoded)
	}

	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := Write(path, want); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Organization != want.Organization || len(loaded.Boxes) != 1 {
		t.Fatalf("loaded manifest = %#v", loaded)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("manifest file missing: %v", err)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load() unexpectedly succeeded for invalid JSON")
	}
}
