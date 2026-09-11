// Package hcp provides access to the HCP Vagrant Box Registry API.
package hcp

import (
	"fmt"
	"net/url"
	"regexp"

	"github.com/go-openapi/strfmt"

	registryclient "github.com/hashicorp/hcp-sdk-go/clients/cloud-vagrant-box-registry/stable/2022-09-30/client"
	"github.com/hashicorp/hcp-sdk-go/clients/cloud-vagrant-box-registry/stable/2022-09-30/client/registry_service"
	"github.com/hashicorp/hcp-sdk-go/clients/cloud-vagrant-box-registry/stable/2022-09-30/models"
	"github.com/hashicorp/hcp-sdk-go/config"
	"github.com/hashicorp/hcp-sdk-go/httpclient"
)

// Credentials are HCP service principal OAuth2 client credentials,
// required to access private registries/boxes. If empty, the client makes
// unauthenticated requests, which works for public registries.
type Credentials struct {
	ClientID     string
	ClientSecret string
}

// These aliases expose the registry models used by the application.
type (
	Registry     = models.HashicorpCloudVagrant20220930Registry
	Box          = models.HashicorpCloudVagrant20220930Box
	Version      = models.HashicorpCloudVagrant20220930Version
	Provider     = models.HashicorpCloudVagrant20220930Provider
	Architecture = models.HashicorpCloudVagrant20220930Architecture
)

// Client wraps the generated HCP Vagrant Box Registry client.
type Client struct {
	sdk *registryclient.CloudVagrantBoxRegistry
}

// NewClient creates an authenticated or anonymous API client.
func NewClient(creds Credentials) (*Client, error) {
	if creds.ClientID == "" || creds.ClientSecret == "" {
		cfg := registryclient.DefaultTransportConfig().WithSchemes([]string{"https"})
		return &Client{sdk: registryclient.NewHTTPClientWithConfig(strfmt.Default, cfg)}, nil
	}

	hcpConfig, err := config.NewHCPConfig(
		config.FromEnv(),
		config.WithClientCredentials(creds.ClientID, creds.ClientSecret),
	)
	if err != nil {
		return nil, fmt.Errorf("building HCP config: %w", err)
	}

	transport, err := httpclient.New(httpclient.Config{HCPConfig: hcpConfig})
	if err != nil {
		return nil, fmt.Errorf("building HCP HTTP client: %w", err)
	}

	return &Client{sdk: registryclient.New(transport, strfmt.Default)}, nil
}

// GetRegistry fetches the expanded registry metadata.
func (c *Client) GetRegistry(registry string) (*Registry, error) {
	expanded := true
	params := registry_service.NewReadRegistryParams().
		WithRegistry(registry).
		WithExpanded(&expanded)

	resp, err := c.sdk.RegistryService.ReadRegistry(params)
	if err != nil {
		return nil, fmt.Errorf("fetching registry %s: %w", registry, err)
	}
	return resp.Payload.Registry, nil
}

// DownloadURL resolves the download URL and checksum for one artifact.
func (c *Client) DownloadURL(registry, box, version, provider, architecture string) (url, checksum, checksumType string, err error) {
	params := registry_service.NewDownloadBoxParams().
		WithRegistry(registry).
		WithBox(box).
		WithVersion(version).
		WithProvider(provider).
		WithArchitecture(architecture)

	resp, err := c.sdk.RegistryService.DownloadBox(params)
	if err != nil {
		return "", "", "", fmt.Errorf("resolving download URL for %s/%s v%s (%s/%s): %w", registry, box, version, provider, architecture, err)
	}

	checksumType = ""
	if resp.Payload.ChecksumType != nil {
		checksumType = string(*resp.Payload.ChecksumType)
	}
	return resp.Payload.URL, resp.Payload.Checksum, checksumType, nil
}

// downloadEndpointBaseURL is the public base URL of the HCP Vagrant Box
// Registry API.
const downloadEndpointBaseURL = "https://api.cloud.hashicorp.com"

// downloadEndpointPattern matches the stable (non-presigned) download
// endpoint URL produced by BuildDownloadURL, capturing its path segments.
var downloadEndpointPattern = regexp.MustCompile(`^/vagrant/2022-09-30/registry/([^/]+)/box/([^/]+)/version/([^/]+)/provider/([^/]+)/architecture/([^/]+)/download$`)

// BuildDownloadURL returns the stable HCP API URL that resolves the
// download location for one box artifact. Unlike the presigned URL
// returned by DownloadURL, this URL does not expire, so it is safe to
// persist in a manifest.
func BuildDownloadURL(registry, box, version, provider, architecture string) string {
	return downloadEndpointBaseURL +
		"/vagrant/2022-09-30/registry/" + url.PathEscape(registry) +
		"/box/" + url.PathEscape(box) +
		"/version/" + url.PathEscape(version) +
		"/provider/" + url.PathEscape(provider) +
		"/architecture/" + url.PathEscape(architecture) +
		"/download"
}

// ParseDownloadURL extracts the registry/box/version/provider/architecture
// path segments from a URL produced by BuildDownloadURL. ok is false if
// downloadURL does not match the expected pattern.
func ParseDownloadURL(downloadURL string) (registry, box, version, provider, architecture string, ok bool) {
	parsed, err := url.Parse(downloadURL)
	if err != nil {
		return "", "", "", "", "", false
	}

	m := downloadEndpointPattern.FindStringSubmatch(parsed.Path)
	if m == nil {
		return "", "", "", "", "", false
	}

	unescape := func(s string) string {
		if d, err := url.PathUnescape(s); err == nil {
			return d
		}
		return s
	}
	return unescape(m[1]), unescape(m[2]), unescape(m[3]), unescape(m[4]), unescape(m[5]), true
}

// ResolveDownloadURL resolves the presigned download URL and checksum for
// one artifact given its stable download endpoint URL, as produced by
// BuildDownloadURL.
func (c *Client) ResolveDownloadURL(downloadURL string) (url, checksum, checksumType string, err error) {
	registry, box, version, provider, architecture, ok := ParseDownloadURL(downloadURL)
	if !ok {
		return "", "", "", fmt.Errorf("download URL %q is not a recognized HCP Vagrant Box Registry download endpoint", downloadURL)
	}
	return c.DownloadURL(registry, box, version, provider, architecture)
}
