package cli

import (
	"github.com/spf13/cobra"
)

// NewCatalogCmd returns the top-level catalog command group.
func NewCatalogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Browse and use workflow templates",
	}
	cmd.AddCommand(newCatalogListCmd())
	cmd.AddCommand(newCatalogSearchCmd())
	cmd.AddCommand(newCatalogInfoCmd())
	cmd.AddCommand(newCatalogInitCmd())
	return cmd
}
