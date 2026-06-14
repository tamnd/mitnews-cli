package cli

import (
	"github.com/spf13/cobra"
)

// searchCmd returns the `search <query>` command.
func (a *App) searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search MIT News articles by keyword",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			n := a.effectiveLimit(20)
			a.progressf("searching MIT News for %q...", query)
			arts, err := a.client.Search(cmd.Context(), query, n)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(arts, len(arts))
		},
	}
}
