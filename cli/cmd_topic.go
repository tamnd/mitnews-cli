package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tamnd/mitnews-cli/mitnews"
)

// topicCmd returns the `topic <name>` command.
func (a *App) topicCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "topic <name>",
		Short:     "Fetch articles from a named MIT News topic feed",
		ValidArgs: mitnews.TopicNames(),
		Args:      cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			topic := args[0]
			n := a.effectiveLimit(20)
			a.progressf("fetching %s articles...", topic)
			arts, err := a.client.TopicArticles(cmd.Context(), topic, n)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(arts, len(arts))
		},
	}
}

// topicsCmd returns the `topics` command listing all available topic feeds.
func (a *App) topicsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "topics",
		Short: "List available MIT News topic feeds",
		RunE: func(cmd *cobra.Command, _ []string) error {
			topics := a.client.Topics()
			if a.output == string(FormatTable) || a.output == "auto" {
				// Print a compact summary if in table mode.
				names := make([]string, len(topics))
				for i, t := range topics {
					names[i] = t.Name
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Available topics: %s\n", strings.Join(names, ", "))
				return nil
			}
			return a.renderOrEmpty(topics, len(topics))
		},
	}
}
