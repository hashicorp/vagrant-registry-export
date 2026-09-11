package hcp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	registryclient "github.com/hashicorp/hcp-sdk-go/clients/cloud-vagrant-box-registry/stable/2022-09-30/client"
)

func TestNewClientAnonymous(t *testing.T) {
	client, err := NewClient(Credentials{})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil || client.sdk == nil {
		t.Fatal("NewClient() returned an uninitialized client")
	}
}

func TestCredentials(t *testing.T) {
	credentials := Credentials{ClientID: "client-id", ClientSecret: "client-secret"}
	if credentials.ClientID == "" || credentials.ClientSecret == "" {
		t.Fatal("credentials should preserve supplied values")
	}
}

func TestGetRegistryAndDownloadURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/vagrant/2022-09-30/registry/example":
			fmt.Fprint(w, `{"registry":{"name":"example","boxes":[]}}`)
		case "/vagrant/2022-09-30/registry/example/box/demo/version/1.0.0/provider/virtualbox/architecture/amd64/download":
			fmt.Fprint(w, `{"url":"https://downloads.example/demo.box","checksum":"abc123","checksum_type":"sha256"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	transport := client.New(server.Listener.Addr().String(), "/", []string{"http"})
	sdk := registryclient.New(transport, strfmt.Default)
	hcpClient := &Client{sdk: sdk}

	registry, err := hcpClient.GetRegistry("example")
	if err != nil {
		t.Fatalf("GetRegistry() error = %v", err)
	}
	if registry.Name != "example" {
		t.Fatalf("registry name = %q, want example", registry.Name)
	}

	url, checksum, checksumType, err := hcpClient.DownloadURL("example", "demo", "1.0.0", "virtualbox", "amd64")
	if err != nil {
		t.Fatalf("DownloadURL() error = %v", err)
	}
	if url != "https://downloads.example/demo.box" || checksum != "abc123" || checksumType != "sha256" {
		t.Fatalf("DownloadURL() = %q, %q, %q", url, checksum, checksumType)
	}
}

func TestBuildAndParseDownloadURL(t *testing.T) {
	got := BuildDownloadURL("example", "demo", "1.0.0", "virtualbox", "amd64")
	want := "https://api.cloud.hashicorp.com/vagrant/2022-09-30/registry/example/box/demo/version/1.0.0/provider/virtualbox/architecture/amd64/download"
	if got != want {
		t.Fatalf("BuildDownloadURL() = %q, want %q", got, want)
	}

	registry, box, version, provider, architecture, ok := ParseDownloadURL(got)
	if !ok || registry != "example" || box != "demo" || version != "1.0.0" || provider != "virtualbox" || architecture != "amd64" {
		t.Fatalf("ParseDownloadURL() = (%q, %q, %q, %q, %q, %t), want (example, demo, 1.0.0, virtualbox, amd64, true)",
			registry, box, version, provider, architecture, ok)
	}

	if _, _, _, _, _, ok := ParseDownloadURL("https://example.com/not-a-download-url"); ok {
		t.Fatal("ParseDownloadURL() ok = true for a non-matching URL, want false")
	}
}

func TestResolveDownloadURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/vagrant/2022-09-30/registry/example/box/demo/version/1.0.0/provider/virtualbox/architecture/amd64/download":
			fmt.Fprint(w, `{"url":"https://downloads.example/demo.box","checksum":"abc123","checksum_type":"sha256"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	transport := client.New(server.Listener.Addr().String(), "/", []string{"http"})
	sdk := registryclient.New(transport, strfmt.Default)
	hcpClient := &Client{sdk: sdk}

	downloadURL := BuildDownloadURL("example", "demo", "1.0.0", "virtualbox", "amd64")
	url, checksum, checksumType, err := hcpClient.ResolveDownloadURL(downloadURL)
	if err != nil {
		t.Fatalf("ResolveDownloadURL() error = %v", err)
	}
	if url != "https://downloads.example/demo.box" || checksum != "abc123" || checksumType != "sha256" {
		t.Fatalf("ResolveDownloadURL() = %q, %q, %q", url, checksum, checksumType)
	}

	if _, _, _, err := hcpClient.ResolveDownloadURL("https://example.com/not-a-download-url"); err == nil {
		t.Fatal("ResolveDownloadURL() error = nil for a non-matching URL, want error")
	}
}
