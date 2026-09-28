// Copyright IBM Corp. 2026

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hashicorp/vagrant-registry-export/internal/exporter"
	"github.com/hashicorp/vagrant-registry-export/internal/hcp"
	"github.com/hashicorp/vagrant-registry-export/internal/manifest"
)

var (
	exportManifestPath string
	exportDestDir      string
	exportForce        bool
	exportConcurrency  int
	exportSkipMetadata bool
	exportBoxes        []string
	exportBoxesAlt     []string
	exportVersions     []string
	exportVersionsAlt  []string
	exportProviders    []string
	exportProvidersAlt []string
	exportArchs        []string
	exportArchsAlt     []string
)

var exportCmd = &cobra.Command{
	Use:   "export [registry]",
	Short: "Download box files and replicate them on disk",
	Long: `Downloads the box files described by a manifest (see "registry
manifest") and replicates them in a local directory tree that mirrors
the registry layout:

  <dest>/<registry>/<box>/versions/<version>/providers/<provider>/<arch>/vagrant.box

Provide either a registry name (to fetch the manifest live from HCP) or
--manifest pointing to a previously generated manifest JSON file.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runExport,
}

func init() {
	exportCmd.Flags().StringVar(&exportManifestPath, "manifest", "", "path to an existing manifest JSON file")
	exportCmd.Flags().StringVarP(&exportDestDir, "dest", "d", "./export", "destination directory to replicate box files into")
	exportCmd.Flags().BoolVar(&exportForce, "force", false, "re-download files even if they already exist on disk")
	exportCmd.Flags().IntVarP(&exportConcurrency, "concurrency", "c", 1, "number of box files to download in parallel")
	exportCmd.Flags().BoolVar(&exportSkipMetadata, "skip-vagrant-metadata", false, `skip writing a per-box "metadata.json" (Vagrant's catalog format) alongside the exported files`)

	exportCmd.Flags().StringSliceVar(&exportBoxes, "box", nil, "limit export to specific box name(s)")
	exportCmd.Flags().StringSliceVar(&exportBoxesAlt, "boxes", nil, "alias for --box, accepts a comma-separated list")
	exportCmd.Flags().StringSliceVar(&exportVersions, "version", nil, "limit export to specific version(s)")
	exportCmd.Flags().StringSliceVar(&exportVersionsAlt, "versions", nil, "alias for --version, accepts a comma-separated list")
	exportCmd.Flags().StringSliceVar(&exportProviders, "provider", nil, "limit export to specific provider(s), e.g. virtualbox")
	exportCmd.Flags().StringSliceVar(&exportProvidersAlt, "providers", nil, "alias for --provider, accepts a comma-separated list")
	exportCmd.Flags().StringSliceVar(&exportArchs, "arch", nil, "limit export to specific architecture(s), e.g. amd64")
	exportCmd.Flags().StringSliceVar(&exportArchsAlt, "archs", nil, "alias for --arch, accepts a comma-separated list")

	registryCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	client, err := hcp.NewClient(resolveCredentials())
	if err != nil {
		return err
	}

	var m *manifest.Manifest
	registryName := ""

	switch {
	case exportManifestPath != "":
		loaded, err := manifest.Load(exportManifestPath)
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}
		m = loaded
	case len(args) == 1:
		registryName = args[0]
		reg, err := client.GetRegistry(registryName)
		if err != nil {
			return err
		}
		m = manifest.FromRegistry(reg)
	default:
		return fmt.Errorf("either a registry name or --manifest must be provided")
	}

	exp := exporter.New(exporter.Options{
		DestDir:     exportDestDir,
		Force:       exportForce,
		Concurrency: exportConcurrency,
		ResolveURL: func(registryName, boxName, version, provider, architecture, downloadURL string) (string, string, string, error) {
			if downloadURL == "" {
				return "", "", "", fmt.Errorf("manifest is missing a download URL for %s v%s (%s/%s)", boxName, version, provider, architecture)
			}
			return client.ResolveDownloadURL(downloadURL)
		},
		SkipVagrantMetadata: exportSkipMetadata,
		Filter: exporter.Filter{
			Boxes:         append(exportBoxes, exportBoxesAlt...),
			Versions:      append(exportVersions, exportVersionsAlt...),
			Providers:     append(exportProviders, exportProvidersAlt...),
			Architectures: append(exportArchs, exportArchsAlt...),
		},
		Progress: func(boxTag, version, provider, architecture string, err error) {
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "FAILED %s v%s (%s/%s): %v\n", boxTag, version, provider, architecture, err)
				return
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "OK     %s v%s (%s/%s)\n", boxTag, version, provider, architecture)
		},
	})

	matched, err := exp.Export(m)
	if err != nil {
		return err
	}

	if matched == 0 {
		return fmt.Errorf("no boxes matched the given filters (box=%v version=%v provider=%v arch=%v)",
			append(exportBoxes, exportBoxesAlt...),
			append(exportVersions, exportVersionsAlt...),
			append(exportProviders, exportProvidersAlt...),
			append(exportArchs, exportArchsAlt...))
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "export complete: %d artifact(s) replicated under %s\n", matched, exportDestDir)
	if !exportSkipMetadata {
		fmt.Fprintf(cmd.ErrOrStderr(), "a metadata.json was written for each box under %s; add a box with e.g. `vagrant box add %s/<registry>/<box>/metadata.json`\n", exportDestDir, exportDestDir)
	}
	return nil
}
