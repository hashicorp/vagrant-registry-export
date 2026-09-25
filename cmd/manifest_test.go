package cmd

import (
	"testing"

	"github.com/hashicorp/vagrant-registry-export/internal/hcp"
)

func TestFilterBoxes(t *testing.T) {
	boxes := []*hcp.Box{
		{Name: "first"},
		nil,
		{Name: "second"},
	}

	got := filterBoxes(boxes, []string{"second"})
	if len(got) != 1 || got[0].Name != "second" {
		t.Fatalf("filterBoxes() = %#v", got)
	}
}

func TestResolveCredentialsFlagPrecedence(t *testing.T) {
	originalID, originalSecret := hcpClientID, hcpSecret
	defer func() {
		hcpClientID, hcpSecret = originalID, originalSecret
	}()

	hcpClientID = "flag-id"
	hcpSecret = "flag-secret"
	if got := resolveCredentials(); got.ClientID != "flag-id" || got.ClientSecret != "flag-secret" {
		t.Fatalf("resolveCredentials() = %#v", got)
	}
}

func TestCommandDefinitions(t *testing.T) {
	if manifestCmd.Args == nil || exportCmd.Args == nil {
		t.Fatal("expected command argument validators")
	}
	for _, name := range []string{"box", "boxes", "version", "versions", "provider", "providers", "arch", "archs"} {
		if exportCmd.Flags().Lookup(name) == nil {
			t.Fatalf("export command missing --%s", name)
		}
	}
}
