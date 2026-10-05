package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var listJSON bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List stored SBOMs",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		s, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer s.Close()

		sboms, err := s.List(cmd.Context())
		if err != nil {
			return err
		}
		if listJSON {
			return writeJSON(cmd, sboms)
		}

		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tNAME\tFORMAT\tCOMPONENTS\tINGESTED\tSOURCE")
		for _, b := range sboms {
			fmt.Fprintf(tw, "%d\t%s\t%s %s\t%d\t%s\t%s\n",
				b.ID, b.Name, b.Format, b.SpecVersion, b.ComponentCount, b.IngestedAt, b.Source)
		}
		return tw.Flush()
	},
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "output JSON")
	rootCmd.AddCommand(listCmd)
}
