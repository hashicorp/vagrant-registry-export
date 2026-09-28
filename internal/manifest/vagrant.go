// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package manifest

import (
	"encoding/json"
	"os"
)

// VagrantMetadata is a single-box catalog document in the format Vagrant
// itself understands (i.e. what "vagrant box add <path-or-url>" expects).
// See https://developer.hashicorp.com/vagrant/docs/boxes/format.
type VagrantMetadata struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description,omitempty"`
	Versions    []VagrantMetadataVersion `json:"versions"`
}

// VagrantMetadataVersion is one version entry within a VagrantMetadata
// document.
type VagrantMetadataVersion struct {
	Version   string                    `json:"version"`
	Providers []VagrantMetadataProvider `json:"providers"`
}

// VagrantMetadataProvider is one provider entry within a
// VagrantMetadataVersion.
type VagrantMetadataProvider struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Architecture string `json:"architecture,omitempty"`
	Checksum     string `json:"checksum,omitempty"`
	ChecksumType string `json:"checksum_type,omitempty"`
}

// MarshalVagrantMetadata returns v as indented JSON bytes.
func MarshalVagrantMetadata(v *VagrantMetadata) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// LoadVagrantMetadata reads and parses a Vagrant-compatible metadata.json
// file from disk. If path does not exist, it returns (nil, nil) so callers
// can distinguish "no existing metadata" from a parse/read error.
func LoadVagrantMetadata(path string) (*VagrantMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var v VagrantMetadata
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
