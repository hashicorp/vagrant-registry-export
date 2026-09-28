// Copyright IBM Corp. 2026

package cmd

import (
	"github.com/spf13/cobra"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Interact with an HCP Vagrant registry",
}
