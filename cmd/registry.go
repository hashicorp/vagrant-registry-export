// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"github.com/spf13/cobra"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Interact with an HCP Vagrant registry",
}
