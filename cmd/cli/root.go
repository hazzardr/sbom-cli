package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "sbom-cli",
	Short: "CLI for working with SBOMs.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

func Execute() error {
	return rootCmd.Execute()
}
