// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package manifest defines the on-disk JSON representation produced by the
// "registry manifest" command and consumed by the "registry export" command.
package manifest

import (
	"encoding/json"
	"os"
	"time"

	"github.com/hashicorp/vagrant-registry-export/internal/hcp"
)

// Manifest is the top-level export manifest for a registry (org/username).
type Manifest struct {
	Organization string    `json:"organization"`
	GeneratedAt  time.Time `json:"generated_at"`
	Boxes        []Box     `json:"boxes"`
}

// Box is a single exported box definition.
type Box struct {
	Tag         string    `json:"tag"`
	Name        string    `json:"name"`
	Private     bool      `json:"private"`
	Description string    `json:"description,omitempty"`
	Versions    []Version `json:"versions"`
}

// Version is a single version of a box, with its downloadable providers.
type Version struct {
	Version     string     `json:"version"`
	Status      string     `json:"status"`
	Description string     `json:"description,omitempty"`
	Providers   []Provider `json:"providers"`
}

// Provider is a single downloadable artifact for a box version.
type Provider struct {
	Name         string `json:"name"`
	Architecture string `json:"architecture"`
	DownloadURL  string `json:"download_url"`
	Checksum     string `json:"checksum,omitempty"`
	ChecksumType string `json:"checksum_type,omitempty"`
}

// FromRegistry converts an HCP registry payload into a Manifest.
func FromRegistry(reg *hcp.Registry) *Manifest {
	m := &Manifest{
		Organization: reg.Name,
		GeneratedAt:  time.Now().UTC(),
		Boxes:        make([]Box, 0, len(reg.Boxes)),
	}

	for _, b := range reg.Boxes {
		if b == nil {
			continue
		}
		mb := Box{
			Tag:         reg.Name + "/" + b.Name,
			Name:        b.Name,
			Private:     b.IsPrivate,
			Description: b.Description,
			Versions:    make([]Version, 0, len(b.Versions)),
		}
		for _, v := range b.Versions {
			if v == nil {
				continue
			}
			var versionState string
			if v.State != nil {
				versionState = string(*v.State)
			}
			mv := Version{
				Version:     v.Name,
				Status:      versionState,
				Description: v.Description,
				Providers:   make([]Provider, 0, len(v.Providers)),
			}
			for _, p := range v.Providers {
				if p == nil {
					continue
				}
				for _, a := range p.Architectures {
					if a == nil {
						continue
					}
					var checksum, checksumType string
					if a.BoxData != nil {
						checksum = a.BoxData.Checksum
						if a.BoxData.ChecksumType != nil {
							checksumType = string(*a.BoxData.ChecksumType)
						}
					}
					mv.Providers = append(mv.Providers, Provider{
						Name:         p.Name,
						Architecture: a.ArchitectureType,
						DownloadURL:  hcp.BuildDownloadURL(reg.Name, b.Name, v.Name, p.Name, a.ArchitectureType),
						Checksum:     checksum,
						ChecksumType: checksumType,
					})
				}
			}
			mb.Versions = append(mb.Versions, mv)
		}
		m.Boxes = append(m.Boxes, mb)
	}

	return m
}

// Write serializes the manifest as indented JSON to the given writer path.
// If path is "-" or empty, it is not written here; callers should handle
// stdout separately.
func Write(path string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// Load reads and parses a manifest JSON file from disk.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Marshal returns the manifest as indented JSON bytes.
func Marshal(m *Manifest) ([]byte, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
