package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/hazzardr/sbom-cli/internal/store"
)

const fixtures = "../../internal/sbom/testdata/"

// run executes the CLI with args and returns stdout. Commands and their
// flags are package-level, so flag values are reset before each run.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetFlags(rootCmd)
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return out.String(), err
}

func resetFlags(cmd *cobra.Command) {
	reset := func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	}
	cmd.Flags().VisitAll(reset)
	cmd.PersistentFlags().VisitAll(reset)
	for _, c := range cmd.Commands() {
		resetFlags(c)
	}
}

func TestUsageLines(t *testing.T) {
	tests := map[string][]string{
		"query": {
			"\n  sbom-cli query --component <name> [--version <version>]\n",
			"\n  sbom-cli query --license <license>\n",
		},
		"ingest": {"\n  sbom-cli ingest <sbom-file>\n"},
	}
	for cmd, want := range tests {
		out, err := run(t, cmd, "--help")
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range want {
			if !strings.Contains(out, line) {
				t.Errorf("%s --help missing usage line %q:\n%s", cmd, strings.TrimSpace(line), out)
			}
		}
	}
}

func TestInvalidInvocations(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"query"}, "one of --component or --license is required"},
		{[]string{"query", "--version", "1.0"}, "--version requires --component"},
		{[]string{"query", "--license", "MIT", "--version", "1.0"}, "--version requires --component"},
		{[]string{"query", "--component", "x", "--license", "MIT"}, "use either --component or --license"},
		{[]string{"query", "--component", ""}, "one of --component or --license is required"},
		{[]string{"query", "extra-arg", "--component", "x"}, "unknown command"},
		{[]string{"ingest"}, "accepts 1 arg(s), received 0"},
		{[]string{"ingest", fixtures + "cyclonedx-1.6.json", fixtures + "spdx-3.0.1.json"}, "accepts 1 arg(s), received 2"},
	}
	for _, tt := range tests {
		_, err := run(t, append(tt.args, "--db", db)...)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%q: want error containing %q, got %v", tt.args, tt.want, err)
		}
	}
}

func TestIngestAndQuery(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	for _, name := range []string{"cyclonedx-1.6.json", "spdx-3.0.1.json"} {
		out, err := run(t, "ingest", fixtures+name, "--db", db)
		if err != nil {
			t.Fatalf("ingest %s: %v", name, err)
		}
		if !strings.Contains(out, "ingested as SBOM") {
			t.Errorf("ingest %s output: %q", name, out)
		}
	}
	out, err := run(t, "ingest", fixtures+"cyclonedx-1.6.json", "--db", db)
	if err != nil || !strings.Contains(out, "already ingested as SBOM 1") {
		t.Errorf("re-ingest: out=%q err=%v", out, err)
	}

	tests := []struct {
		args []string
		want []string
	}{
		{[]string{"--component", "log4j-core"}, []string{"log4j-core@2.14.1", "log4j-core@2.17.1"}},
		{[]string{"--component", "log4j-core", "--version", "2.14.1"}, []string{"log4j-core@2.14.1"}},
		{[]string{"--license", "MIT"}, []string{"hyper@0.14.28", "serde@1.0.210"}},
		{[]string{"--component", "does-not-exist"}, nil},
	}
	for _, tt := range tests {
		out, err := run(t, append([]string{"query", "--json", "--db", db}, tt.args...)...)
		if err != nil {
			t.Fatalf("query %q: %v", tt.args, err)
		}
		var matches []store.Match
		if err := json.Unmarshal([]byte(out), &matches); err != nil {
			t.Fatalf("query %q: invalid JSON %q: %v", tt.args, out, err)
		}
		var got []string
		for _, m := range matches {
			got = append(got, m.Component+"@"+m.Version)
		}
		slices.Sort(got)
		if !slices.Equal(got, tt.want) {
			t.Errorf("query %q: got %v, want %v", tt.args, got, tt.want)
		}
	}

	out, err = run(t, "query", "--component", "serde", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "SBOM") || !strings.Contains(out, "MIT OR Apache-2.0") {
		t.Errorf("table output:\n%s", out)
	}
}
