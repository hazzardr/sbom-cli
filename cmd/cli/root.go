package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/hazzardr/sbom-cli/internal/store"
)

const defaultDBPath = "data/sbom-cli.db"

var dbPath string

var rootCmd = &cobra.Command{
	Use:   "sbom-cli",
	Short: "Ingest, store, and query SBOMs (CycloneDX 1.6/1.7 and SPDX 3.0 JSON).",
	// main logs returned errors; usage is only useful for flag errors.
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

func init() {
	defaultDB := os.Getenv("DB_URL")
	if defaultDB == "" {
		defaultDB = defaultDBPath
	}
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDB,
		"path to the SQLite database (defaults to $DB_URL)")
}

func Execute() error {
	return rootCmd.Execute()
}

func openStore(cmd *cobra.Command) (*store.Store, error) {
	return store.Open(cmd.Context(), dbPath)
}
