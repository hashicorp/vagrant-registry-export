package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	buildversion "github.com/hashicorp/vagrant-registry-export/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version and build information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "vagrant-registry-export v%s\n", buildversion.FormattedVersion())
	},
}
