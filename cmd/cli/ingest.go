package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var ingestCmd = &cobra.Command{
	Use:   "ingest FILE...",
	Short: "Ingest CycloneDX 1.6 or SPDX 3.0 JSON SBOMs (use - for stdin)",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer s.Close()

		out := cmd.OutOrStdout()
		var errs []error
		for _, path := range args {
			raw, err := readInput(cmd, path)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			res, err := s.Ingest(cmd.Context(), path, raw)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", path, err))
				continue
			}
			if res.Duplicate {
				fmt.Fprintf(out, "%s: already ingested as SBOM %d\n", path, res.ID)
				continue
			}
			fmt.Fprintf(out, "%s: ingested as SBOM %d (%s, %d components)\n",
				path, res.ID, res.Format, res.Components)
		}
		return errors.Join(errs...)
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
