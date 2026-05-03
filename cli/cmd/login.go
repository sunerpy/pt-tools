package cmd

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/cli/internal/ui"
)

var loginCmd = &cobra.Command{
	Use:   "login [username] [password]",
	Short: "Login to pt-tools server",
	Long: `Authenticate with the pt-tools server and cache the session cookie.
If username/password are not provided, you will be prompted.

Examples:
  pt-tools-cli login admin adminadmin
  pt-tools-cli login admin   # prompts for password
  pt-tools-cli login         # prompts for both`,
	RunE: runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)
}

func runLogin(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}

	var username, password string

	switch len(args) {
	case 2:
		username = args[0]
		password = args[1]
	case 1:
		username = args[0]
		var err error
		password, err = ui.Password("Password:")
		if err != nil {
			return err
		}
	default:
		var err error
		if cfg.Username != "" {
			username = cfg.Username
		} else if u := os.Getenv("PT_TOOLS_USER"); u != "" {
			username = u
		} else {
			username, err = ui.Input("Username:", "admin")
			if err != nil {
				return err
			}
		}
		password, err = ui.Password("Password:")
		if err != nil {
			return err
		}
	}

	if err := apiClient.Login(username, password); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	color.Green("✓ Logged in as %s to %s", username, cfg.URL)
	return nil
}
