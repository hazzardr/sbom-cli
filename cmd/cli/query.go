package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hazzardr/sbom-cli/internal/store"
)

var (
	queryFilter store.Filter
	queryJSON   bool
)

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Find components by name, version, and/or license across all SBOMs",
	Long: `Find components by name, version, and/or license across all SBOMs.

All given filters must match. Component names and licenses match
case-insensitively; versions match exactly. A license matches any SPDX
expression that references it, so --license MIT finds "MIT OR Apache-2.0".`,
	Example: `  sbom-cli query --component log4j-core
  sbom-cli query --component log4j-core --version 2.14.1
  sbom-cli query --license GPL-3.0-only --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		s, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer s.Close()

		matches, err := s.Search(cmd.Context(), queryFilter)
		if err != nil {
			return err
		}
		if queryJSON {
			return writeJSON(cmd, matches)
		}

		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SBOM\tCOMPONENT\tVERSION\tLICENSES\tPURL")
		for _, m := range matches {
			fmt.Fprintf(tw, "%d (%s)\t%s\t%s\t%s\t%s\n",
				m.SBOMID, m.SBOMName, m.Component, m.Version, m.Licenses, m.PURL)
		}
		return tw.Flush()
	},
}

// writeJSON writes v as indented JSON, rendering nil slices as [].
func writeJSON[T any](cmd *cobra.Command, v []T) error {
	if v == nil {
		v = []T{}
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func init() {
	f := queryCmd.Flags()
	f.StringVar(&queryFilter.Component, "component", "", "component name")
	f.StringVar(&queryFilter.Version, "version", "", "component version")
	f.StringVar(&queryFilter.License, "license", "", "license ID or name")
	f.BoolVar(&queryJSON, "json", false, "output JSON")
	queryCmd.MarkFlagsOneRequired("component", "version", "license")
	rootCmd.AddCommand(queryCmd)
}
