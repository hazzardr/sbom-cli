package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:                   "show <id>",
	Short:                 "Print a stored SBOM document as JSON",
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid SBOM ID %q", args[0])
		}
		s, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer s.Close()

		doc, err := s.Document(cmd.Context(), id)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(doc), "", "  "); err != nil {
			return fmt.Errorf("format SBOM %d: %w", id, err)
		}
		buf.WriteByte('\n')
		_, err = buf.WriteTo(cmd.OutOrStdout())
		return err
	},
}

func init() {
	rootCmd.AddCommand(showCmd)
}
