package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var ingestCmd = &cobra.Command{
	Use:                   "ingest <sbom-file>",
	Short:                 "Ingest a CycloneDX 1.6/1.7 or SPDX 3.0 JSON SBOM (use - for stdin)",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]
		raw, err := readInput(cmd, path)
		if err != nil {
			return err
		}
		s, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer s.Close()

		res, err := s.Ingest(cmd.Context(), path, raw)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		out := cmd.OutOrStdout()
		if res.Duplicate {
			fmt.Fprintf(out, "%s: already ingested as SBOM %d\n", path, res.ID)
			return nil
		}
		fmt.Fprintf(out, "%s: ingested as SBOM %d (%s, %d components)\n",
			path, res.ID, res.Format, res.Components)
		return nil
	},
}

func readInput(cmd *cobra.Command, path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

func init() {
	rootCmd.AddCommand(ingestCmd)
}
