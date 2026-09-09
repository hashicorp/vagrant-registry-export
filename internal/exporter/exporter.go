// Package exporter downloads box files described by a manifest and
// writes them to a registry-organized directory.
package exporter

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hashicorp/vagrant-registry-export/internal/manifest"
)

// Filter optionally restricts which box/version/provider/architecture
// artifacts are exported. Each field is an allow-list: empty means "no
// restriction" for that dimension. All non-empty dimensions must match for
// an artifact to be exported.
type Filter struct {
	Boxes         []string
	Versions      []string
	Providers     []string
	Architectures []string
}

// matches reports whether the given artifact coordinates pass the filter.
func (f Filter) matches(box, version, provider, architecture string) bool {
	return matchesAny(f.Boxes, box) &&
		matchesAny(f.Versions, version) &&
		matchesAny(f.Providers, provider) &&
		matchesAny(f.Architectures, architecture)
}

// matchesAny reports whether value is in allowed, or allowed is empty
// (meaning "allow anything").
func matchesAny(allowed []string, value string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if strings.EqualFold(a, value) {
			return true
		}
	}
	return false
}

// Options controls export behavior.
type Options struct {
	DestDir string
	// ResolveURL resolves a download URL and checksum for one artifact.
	ResolveURL func(registryName, boxName, version, provider, architecture string) (url, checksum, checksumType string, err error)
	// Filter, if set, restricts which artifacts are downloaded.
	Filter Filter
	// Force re-downloads files that already exist on disk.
	Force bool
	// Concurrency controls how many provider artifacts are downloaded in
	// parallel. Defaults to 1 if <= 0.
	Concurrency int
	// Progress, if set, is called after each provider artifact finishes.
	Progress func(boxTag, version, provider, architecture string, err error)
}

// Exporter downloads manifest-described box files to a destination directory.
type Exporter struct {
	opts       Options
	httpClient *http.Client
}

// New creates an Exporter with the given options.
func New(opts Options) *Exporter {
	return &Exporter{
		opts: opts,
		httpClient: &http.Client{
			Timeout: 0, // box files can be large; rely on per-request context if needed
		},
	}
}

// Export walks the manifest and downloads every provider artifact that is
// not already present on disk (unless Force is set). Downloads run
// concurrently according to Options.Concurrency.
func (e *Exporter) Export(m *manifest.Manifest) error {
	type job struct {
		box          manifest.Box
		version      manifest.Version
		provider     manifest.Provider
		architecture string
	}

	var jobs []job
	for _, box := range m.Boxes {
		for _, version := range box.Versions {
			for _, provider := range version.Providers {
				if !e.opts.Filter.matches(box.Name, version.Version, provider.Name, provider.Architecture) {
					continue
				}
				jobs = append(jobs, job{
					box:          box,
					version:      version,
					provider:     provider,
					architecture: provider.Architecture,
				})
			}
		}
	}

	concurrency := e.opts.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	jobCh := make(chan job)
	errCh := make(chan error, len(jobs))
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				err := e.exportArtifact(m.Organization, j.box, j.version, j.provider, j.architecture)
				if e.opts.Progress != nil {
					e.opts.Progress(j.box.Tag, j.version.Version, j.provider.Name, j.architecture, err)
				}
				if err != nil {
					errCh <- fmt.Errorf("exporting %s v%s (%s/%s): %w", j.box.Tag, j.version.Version, j.provider.Name, j.architecture, err)
				}
			}
		}()
	}

	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)
	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (e *Exporter) exportArtifact(registryName string, box manifest.Box, version manifest.Version, provider manifest.Provider, architecture string) error {
	if e.opts.ResolveURL == nil {
		return fmt.Errorf("no URL resolver configured for provider %q", provider.Name)
	}

	destPath := e.artifactPath(box, version, provider, architecture)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	if !e.opts.Force {
		if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
			// Already downloaded; verify checksum if we can, otherwise trust it.
			if provider.Checksum == "" {
				return nil
			}
			if ok, err := verifyChecksum(destPath, provider.ChecksumType, provider.Checksum); err == nil && ok {
				return nil
			}
			// Checksum mismatch or verification error: re-download.
		}
	}

	downloadURL, checksum, checksumType, err := e.opts.ResolveURL(registryName, box.Name, version.Version, provider.Name, architecture)
	if err != nil {
		return fmt.Errorf("resolving download URL: %w", err)
	}
	if checksum != "" {
		provider.Checksum = checksum
		provider.ChecksumType = checksumType
	}

	tmpPath := destPath + ".tmp"
	if err := e.download(downloadURL, tmpPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if provider.Checksum != "" {
		ok, err := verifyChecksum(tmpPath, provider.ChecksumType, provider.Checksum)
		if err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("verifying checksum: %w", err)
		}
		if !ok {
			os.Remove(tmpPath)
			return fmt.Errorf("checksum mismatch for %s", downloadURL)
		}
	}

	return os.Rename(tmpPath, destPath)
}

func (e *Exporter) download(rawURL, destPath string) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d downloading %s", resp.StatusCode, rawURL)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	return nil
}

// artifactPath returns the local path for one box artifact.
func (e *Exporter) artifactPath(box manifest.Box, version manifest.Version, provider manifest.Provider, architecture string) string {
	arch := architecture
	if arch == "" {
		arch = "unknown"
	}
	tag := strings.ReplaceAll(box.Tag, "/", string(filepath.Separator))
	return filepath.Join(
		e.opts.DestDir,
		tag,
		"versions", version.Version,
		"providers", provider.Name,
		arch,
		"vagrant.box",
	)
}

func verifyChecksum(path, checksumType, want string) (bool, error) {
	var h hash.Hash
	switch strings.ToLower(checksumType) {
	case "sha1":
		h = sha1.New()
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	default:
		// Unknown or empty checksum type: nothing to verify against.
		return true, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}

	got := hex.EncodeToString(h.Sum(nil))
	return strings.EqualFold(got, want), nil
}
