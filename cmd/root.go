// Package cmd implements the vagrant-registry-export CLI commands.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/hashicorp/vagrant-registry-export/internal/hcp"
)

var (
	cfgFile     string
	hcpClientID string
	hcpSecret   string
)

// Execute runs the CLI.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:          "vagrant-registry-export",
	Short:        "Export HCP Vagrant registries",
	Long:         "Generate registry manifests and download box artifacts locally.",
	SilenceUsage: true,
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: $HOME/.vagrant-registry-export.yaml)")
	rootCmd.PersistentFlags().StringVar(&hcpClientID, "hcp-client-id", "", "HCP service principal client ID (overrides HCP_CLIENT_ID env var and config file)")
	rootCmd.PersistentFlags().StringVar(&hcpSecret, "hcp-client-secret", "", "HCP service principal client secret (overrides HCP_CLIENT_SECRET env var and config file)")

	rootCmd.AddCommand(registryCmd)
}

func initConfig() {
	viper.SetEnvPrefix("HCP")
	viper.AutomaticEnv()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(home)
		}
		viper.AddConfigPath(".")
		viper.SetConfigName(".vagrant-registry-export")
		viper.SetConfigType("yaml")
	}

	// Ignore error: config file is optional.
	_ = viper.ReadInConfig()
}

// resolveCredentials returns HCP service principal credentials using the
// precedence: --hcp-client-id/--hcp-client-secret flags >
// HCP_CLIENT_ID/HCP_CLIENT_SECRET env vars > config file keys.
func resolveCredentials() hcp.Credentials {
	id := hcpClientID
	if id == "" {
		id = viper.GetString("client_id")
	}
	secret := hcpSecret
	if secret == "" {
		secret = viper.GetString("client_secret")
	}
	return hcp.Credentials{ClientID: id, ClientSecret: secret}
}
