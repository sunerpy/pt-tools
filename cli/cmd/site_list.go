package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/cli/internal/output"
)

var siteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sites",
	RunE:  runSiteList,
}

var siteDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a site",
	Args:  cobra.ExactArgs(1),
	RunE:  runSiteDelete,
}

var siteValidateCmd = &cobra.Command{
	Use:   "validate <name>",
	Short: "Validate site credentials",
	Args:  cobra.ExactArgs(1),
	RunE:  runSiteValidate,
}

func init() {
	siteCmd.AddCommand(siteListCmd)
	siteCmd.AddCommand(siteDeleteCmd)
	siteCmd.AddCommand(siteValidateCmd)
}

func runSiteList(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}
	if err := requireAuth(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	sites, err := apiClient.ListSites(ctx)
	if err != nil {
		return err
	}

	rows := make([][]any, 0, len(sites))
	for name, site := range sites {
		status := "disabled"
		if site.Enabled != nil && *site.Enabled {
			status = "enabled"
		}
		if site.Unavailable {
			status = "unavailable"
		}

		rows = append(rows, []any{
			name,
			site.AuthMethod,
			status,
		})
	}

	output.PrintTable([]string{"Site", "Auth", "Status"}, rows)
	fmt.Printf("\nTotal: %d sites\n", len(sites))
	return nil
}

func runSiteDelete(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}
	if err := requireAuth(); err != nil {
		return err
	}

	name := args[0]

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	if err := apiClient.DeleteSite(ctx, name); err != nil {
		return err
	}

	output.Success("Site '%s' deleted.", name)
	return nil
}

func runSiteValidate(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}
	if err := requireAuth(); err != nil {
		return err
	}

	name := args[0]

	output.Spinner(fmt.Sprintf("Validating site '%s'...", name))

	ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
	defer cancel()

	if err := apiClient.ValidateSite(ctx, name); err != nil {
		output.Done()
		return err
	}
	output.Done()

	color.Green("Site '%s' validated successfully.", name)
	return nil
}
