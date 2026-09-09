package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/hashicorp/vagrant-registry-export/internal/hcp"
	"github.com/hashicorp/vagrant-registry-export/internal/manifest"
)

var (
	manifestOutput string
	manifestBoxes  []string
)

var manifestCmd = &cobra.Command{
	Use:   "manifest <registry>",
	Short: "Generate a JSON manifest of a registry's boxes and versions",
	Long: `Fetches box, version, provider, and architecture metadata for the
given registry from the HCP Vagrant Registry API and prints or writes a JSON
manifest for use with "registry export".`,
	Args: cobra.ExactArgs(1),
	RunE: runManifest,
}

func init() {
	manifestCmd.Flags().StringVarP(&manifestOutput, "output", "o", "-", `file to write the manifest to ("-" for stdout)`)
	manifestCmd.Flags().StringSliceVar(&manifestBoxes, "box", nil, "limit the manifest to specific box names within the registry (repeatable, e.g. --box bionic64 --box precise64)")
	registryCmd.AddCommand(manifestCmd)
}

func runManifest(cmd *cobra.Command, args []string) error {
	registryName := args[0]
	client, err := hcp.NewClient(resolveCredentials())
	if err != nil {
		return err
	}

	reg, err := client.GetRegistry(registryName)
	if err != nil {
		return err
	}

	if len(manifestBoxes) > 0 {
		reg.Boxes = filterBoxes(reg.Boxes, manifestBoxes)
	}

	m := manifest.FromRegistry(reg)

	if manifestOutput == "-" || manifestOutput == "" {
		data, err := manifest.Marshal(m)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(data)
		return err
	}

	if err := manifest.Write(manifestOutput, m); err != nil {
		return err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "wrote manifest for %d box(es) to %s\n", len(m.Boxes), manifestOutput)
	return nil
}

// filterBoxes keeps only boxes whose Name is in the given allow-list.
func filterBoxes(boxes []*hcp.Box, names []string) []*hcp.Box {
	allow := make(map[string]bool, len(names))
	for _, n := range names {
		allow[n] = true
	}
	filtered := make([]*hcp.Box, 0, len(names))
	for _, b := range boxes {
		if b != nil && allow[b.Name] {
			filtered = append(filtered, b)
		}
	}
	return filtered
}
