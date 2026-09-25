// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package exporter downloads box files described by a manifest and
// writes them to a registry-organized directory.
package exporter

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
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
	// downloadURL is the artifact's manifest.Provider.DownloadURL, if set.
	ResolveURL func(registryName, boxName, version, provider, architecture, downloadURL string) (url, checksum, checksumType string, err error)
	// Filter, if set, restricts which artifacts are downloaded.
	Filter Filter
	// Force re-downloads files that already exist on disk.
	Force bool
	// Concurrency controls how many provider artifacts are downloaded in
	// parallel. Defaults to 1 if <= 0.
	Concurrency int
	// Progress, if set, is called after each provider artifact finishes.
	Progress func(boxTag, version, provider, architecture string, err error)
	// SkipVagrantMetadata disables writing a per-box Vagrant-compatible
	// metadata.json alongside the downloaded artifacts.
	SkipVagrantMetadata bool
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
			// No client-wide timeout: box files can be many gigabytes and
			// a fixed deadline would abort otherwise-healthy downloads.
			Timeout: 0,
		},
	}
}

// exportJob describes one box artifact queued for export.
type exportJob struct {
	box          manifest.Box
	version      manifest.Version
	provider     manifest.Provider
	architecture string
}

// exportResult is the outcome of processing one exportJob.
type exportResult struct {
	exportJob
	path         string
	checksum     string
	checksumType string
	err          error
}

// Export walks the manifest and downloads every provider artifact that
// matches Options.Filter and is not already present on disk (unless Force
// is set). It returns the number of artifacts that matched the filter,
// regardless of whether they were newly downloaded or already on disk.
//
// Unless Options.SkipVagrantMetadata is set, a per-box metadata.json in
// Vagrant's own catalog format is also written, so the result can be added
// with "vagrant box add <path-to-metadata.json>".
func (e *Exporter) Export(m *manifest.Manifest) (int, error) {
	var jobs []exportJob
	for _, box := range m.Boxes {
		for _, version := range box.Versions {
			for _, provider := range version.Providers {
				if !e.opts.Filter.matches(box.Name, version.Version, provider.Name, provider.Architecture) {
					continue
				}
				jobs = append(jobs, exportJob{
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

	jobCh := make(chan exportJob)
	resultCh := make(chan exportResult, len(jobs))
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				path, checksum, checksumType, err := e.exportArtifact(m.Organization, j.box, j.version, j.provider, j.architecture)
				if e.opts.Progress != nil {
					e.opts.Progress(j.box.Tag, j.version.Version, j.provider.Name, j.architecture, err)
				}
				if err != nil {
					err = fmt.Errorf("exporting %s v%s (%s/%s): %w", j.box.Tag, j.version.Version, j.provider.Name, j.architecture, err)
				}
				resultCh <- exportResult{exportJob: j, path: path, checksum: checksum, checksumType: checksumType, err: err}
			}
		}()
	}

	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)
	wg.Wait()
	close(resultCh)

	var errs []error
	var results []exportResult
	for r := range resultCh {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		results = append(results, r)
	}

	if !e.opts.SkipVagrantMetadata {
		if err := e.writeVagrantMetadata(results); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return len(jobs), errors.Join(errs...)
	}
	return len(jobs), nil
}

// exportArtifact downloads (or verifies an already-present) box file for
// one artifact and returns its local path and checksum information.
func (e *Exporter) exportArtifact(registryName string, box manifest.Box, version manifest.Version, provider manifest.Provider, architecture string) (path, checksum, checksumType string, err error) {
	if e.opts.ResolveURL == nil {
		return "", "", "", fmt.Errorf("no URL resolver configured for provider %q", provider.Name)
	}

	destPath := e.artifactPath(box, version, provider, architecture)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", "", "", fmt.Errorf("creating directory: %w", err)
	}

	if !e.opts.Force {
		if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
			// Already downloaded; verify checksum if we can, otherwise trust it.
			if provider.Checksum == "" {
				return destPath, "", "", nil
			}
			if ok, err := verifyChecksum(destPath, provider.ChecksumType, provider.Checksum); err == nil && ok {
				return destPath, provider.Checksum, provider.ChecksumType, nil
			}
			// Checksum mismatch or verification error: re-download.
		}
	}

	resolvedURL, resolvedChecksum, resolvedChecksumType, err := e.opts.ResolveURL(registryName, box.Name, version.Version, provider.Name, architecture, provider.DownloadURL)
	if err != nil {
		return "", "", "", fmt.Errorf("resolving download URL: %w", err)
	}
	if resolvedChecksum != "" {
		checksum = resolvedChecksum
		checksumType = resolvedChecksumType
	} else {
		checksum = provider.Checksum
		checksumType = provider.ChecksumType
	}

	tmpPath := destPath + ".tmp"
	if err := e.download(resolvedURL, tmpPath); err != nil {
		os.Remove(tmpPath)
		return "", "", "", err
	}

	if checksum != "" {
		ok, err := verifyChecksum(tmpPath, checksumType, checksum)
		if err != nil {
			os.Remove(tmpPath)
			return "", "", "", fmt.Errorf("verifying checksum: %w", err)
		}
		if !ok {
			os.Remove(tmpPath)
			return "", "", "", fmt.Errorf("checksum mismatch for %s", resolvedURL)
		}
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return "", "", "", err
	}
	return destPath, checksum, checksumType, nil
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

// vagrantMetadataPath returns the path to the per-box Vagrant-compatible
// metadata.json.
func (e *Exporter) vagrantMetadataPath(box manifest.Box) string {
	tag := strings.ReplaceAll(box.Tag, "/", string(filepath.Separator))
	return filepath.Join(e.opts.DestDir, tag, "metadata.json")
}

// writeVagrantMetadata groups successfully exported artifacts by box and
// writes a Vagrant-compatible metadata.json for each, using "file://" URLs
// pointing at the locally downloaded box files.
//
// Any existing metadata.json for a box is loaded and merged with the new
// results (keyed by version+provider+architecture), so exporting with
// different filters across multiple runs accumulates entries instead of
// overwriting them.
func (e *Exporter) writeVagrantMetadata(results []exportResult) error {
	boxes := map[string]manifest.Box{}
	// box tag -> version -> provider+arch key -> provider
	versions := map[string]map[string]map[string]manifest.VagrantMetadataProvider{}

	providerKey := func(name, arch string) string { return name + "/" + arch }

	for _, r := range results {
		boxes[r.box.Tag] = r.box
		if versions[r.box.Tag] == nil {
			versions[r.box.Tag] = map[string]map[string]manifest.VagrantMetadataProvider{}
		}
		if versions[r.box.Tag][r.version.Version] == nil {
			versions[r.box.Tag][r.version.Version] = map[string]manifest.VagrantMetadataProvider{}
		}

		absPath, err := filepath.Abs(r.path)
		if err != nil {
			absPath = r.path
		}

		versions[r.box.Tag][r.version.Version][providerKey(r.provider.Name, r.architecture)] = manifest.VagrantMetadataProvider{
			Name:         r.provider.Name,
			URL:          fileURL(absPath),
			Architecture: r.architecture,
			Checksum:     r.checksum,
			ChecksumType: r.checksumType,
		}
	}

	tags := make([]string, 0, len(boxes))
	for tag := range boxes {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	for _, tag := range tags {
		box := boxes[tag]
		dest := e.vagrantMetadataPath(box)

		existing, err := manifest.LoadVagrantMetadata(dest)
		if err != nil {
			return fmt.Errorf("reading existing vagrant metadata for %s: %w", tag, err)
		}
		if existing != nil {
			for _, v := range existing.Versions {
				if versions[tag][v.Version] == nil {
					versions[tag][v.Version] = map[string]manifest.VagrantMetadataProvider{}
				}
				for _, p := range v.Providers {
					key := providerKey(p.Name, p.Architecture)
					// Keep the on-disk entry unless this run re-exported it.
					if _, exists := versions[tag][v.Version][key]; !exists {
						versions[tag][v.Version][key] = p
					}
				}
			}
		}

		versionNames := make([]string, 0, len(versions[tag]))
		for v := range versions[tag] {
			versionNames = append(versionNames, v)
		}
		sort.Strings(versionNames)

		md := &manifest.VagrantMetadata{
			Name:        box.Tag,
			Description: box.Description,
		}
		if md.Description == "" && existing != nil {
			md.Description = existing.Description
		}
		for _, v := range versionNames {
			providers := make([]manifest.VagrantMetadataProvider, 0, len(versions[tag][v]))
			for _, p := range versions[tag][v] {
				providers = append(providers, p)
			}
			sort.Slice(providers, func(i, j int) bool {
				if providers[i].Name != providers[j].Name {
					return providers[i].Name < providers[j].Name
				}
				return providers[i].Architecture < providers[j].Architecture
			})
			md.Versions = append(md.Versions, manifest.VagrantMetadataVersion{
				Version:   v,
				Providers: providers,
			})
		}

		data, err := manifest.MarshalVagrantMetadata(md)
		if err != nil {
			return fmt.Errorf("marshaling vagrant metadata for %s: %w", tag, err)
		}

		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("creating directory for vagrant metadata: %w", err)
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return fmt.Errorf("writing vagrant metadata for %s: %w", tag, err)
		}
	}

	return nil
}

// fileURL converts an absolute filesystem path into a "file://" URL usable
// as a provider URL with "vagrant box add".
func fileURL(path string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	return u.String()
}

func verifyChecksum(path, checksumType, want string) (bool, error) {
	var h hash.Hash
	switch strings.ToLower(checksumType) {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	case "sha256":
		h = sha256.New()
	case "sha384":
		h = sha512.New384()
	case "sha512":
		h = sha512.New()
	case "", "none", "unset":
		// No checksum available: nothing to verify against.
		return true, nil
	default:
		if want == "" {
			return true, nil
		}
		// A checksum was provided but we cannot validate it; fail closed
		// rather than silently trusting an unverifiable download.
		return false, fmt.Errorf("unsupported checksum type %q", checksumType)
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
