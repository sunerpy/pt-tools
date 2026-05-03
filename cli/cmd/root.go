package cmd

import (
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/cli/internal/client"
	"github.com/sunerpy/pt-tools/cli/internal/config"
	"github.com/sunerpy/pt-tools/cli/internal/types"
)

var (
	urlFlag    string
	insecure   bool
	cfg        *types.CLIConfig
	apiClient  *client.Client
)

var rootCmd = &cobra.Command{
	Use:   "pt-tools-cli",
	Short: "Remote CLI for pt-tools Docker instance",
	Long: `pt-tools-cli provides remote management of a pt-tools Docker deployment
via its web API. Use 'login' to authenticate, then use commands like 'search',
'site', 'task', 'downloader', etc.

Examples:
  pt-tools-cli --url http://localhost:8080 login
  pt-tools-cli search "Movie Name"
  pt-tools-cli site list
  pt-tools-cli task list
  pt-tools-cli downloader list

Environment variables:
  PT_TOOLS_URL    Server URL (alternative to --url)
  PT_TOOLS_USER   Default username for login`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&urlFlag, "url", "", "pt-tools server URL (e.g., http://localhost:8080)")
	rootCmd.PersistentFlags().BoolVar(&insecure, "insecure", false, "Skip TLS certificate verification")
}

// initClient loads config and initializes the API client.
func initClient() error {
	var err error
	cfg, err = config.Load()
	if err != nil {
		return err
	}

	// URL priority: flag > env > config
	if urlFlag != "" {
		cfg.URL = urlFlag
	} else if u := os.Getenv("PT_TOOLS_URL"); u != "" {
		cfg.URL = u
	}

	if cfg.URL == "" {
		return client.ErrNotAuthenticated
	}

	// Set username from env if not set
	if cfg.Username == "" {
		if u := os.Getenv("PT_TOOLS_USER"); u != "" {
			cfg.Username = u
		}
	}

	apiClient = client.NewClient(cfg)
	apiClient.SetInsecure(insecure)

	return nil
}

// requireAuth ensures the client has a valid session.
func requireAuth() error {
	if !apiClient.IsAuthenticated() {
		color.Red("Not authenticated. Run 'pt-tools-cli login' first.")
		return client.ErrNotAuthenticated
	}
	return nil
}
