package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/cli/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
	Long:  `View and modify CLI configuration settings.`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE:  runConfigShow,
}

var configClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear cached session cookie",
	RunE:  runConfigClear,
}

var configSetURLCmd = &cobra.Command{
	Use:   "set-url <url>",
	Short: "Set server URL",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigSetURL,
}

var configSetUserCmd = &cobra.Command{
	Use:   "set-user <username>",
	Short: "Set default username",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigSetUser,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configClearCmd)
	configCmd.AddCommand(configSetURLCmd)
	configCmd.AddCommand(configSetUserCmd)
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	color.Cyan("CLI Configuration:")
	fmt.Printf("  Server URL: %s\n", cfg.URL)
	fmt.Printf("  Username:   %s\n", cfg.Username)
	if cfg.Cookie != "" {
		fmt.Printf("  Session:    %s (cached)\n", cfg.Cookie[:8]+"...")
	} else {
		fmt.Printf("  Session:    not logged in\n")
	}
	if cfg.Expires > 0 {
		fmt.Printf("  Expires:    %d\n", cfg.Expires)
	}

	dir, _ := config.ConfigDir()
	fmt.Printf("  Config Dir: %s\n", dir)
	return nil
}

func runConfigClear(cmd *cobra.Command, args []string) error {
	if err := config.Clear(); err != nil {
		return err
	}
	color.Green("Session cleared.")
	return nil
}

func runConfigSetURL(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.URL = args[0]
	if err := config.Save(cfg); err != nil {
		return err
	}
	color.Green("Server URL set to: %s", args[0])
	return nil
}

func runConfigSetUser(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Username = args[0]
	if err := config.Save(cfg); err != nil {
		return err
	}
	color.Green("Username set to: %s", args[0])
	return nil
}
