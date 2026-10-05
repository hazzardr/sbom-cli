package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hazzardr/sbom-cli/internal/sbom"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "sbom", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// fixtures are ingested in this order, so SBOM IDs are 1-5 respectively.
var fixtures = []string{
	"cyclonedx-1.6-spec-valid-bom.json",
	"cyclonedx-1.7-guide-bom-link.json",
	"cyclonedx-1.7-spec-license-choice.json",
	"spdx-3.0.1-examples-example11.json",
	"spdx-3.0.1-spec-package-sbom.json",
}

func ingestFixtures(t *testing.T, s *Store) {
	t.Helper()
	for _, name := range fixtures {
		if _, err := s.Ingest(t.Context(), name, readFixture(t, name)); err != nil {
			t.Fatalf("ingest %s: %v", name, err)
		}
	}
}

func TestIngestDeduplicates(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)
	raw := readFixture(t, "cyclonedx-1.6-spec-valid-bom.json")

	first, err := s.Ingest(t.Context(), "a.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	if first.Duplicate || first.Components != 4 {
		t.Errorf("first ingest: %+v", first)
	}
	second, err := s.Ingest(t.Context(), "b.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate || second.ID != first.ID {
		t.Errorf("second ingest should be a duplicate of %d: %+v", first.ID, second)
	}

	list, err := s.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ComponentCount != 4 || list[0].Name != "Acme Application" {
		t.Errorf("list: %+v", list)
	}
}

func TestIngestRejectsUnsupported(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)
	raw := readFixture(t, "unsupported/cyclonedx-1.4-examples-laravel.json")
	if _, err := s.Ingest(t.Context(), "laravel.json", raw); !errors.Is(err, sbom.ErrUnsupportedFormat) {
		t.Fatalf("want ErrUnsupportedFormat for CycloneDX 1.4, got %v", err)
	}
	list, err := s.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("nothing should be stored, got %+v", list)
	}
}

func TestDocumentStoredAsJSONB(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)
	raw := readFixture(t, "spdx-3.0.1-examples-example11.json")
	res, err := s.Ingest(t.Context(), "spdx.json", raw)
	if err != nil {
		t.Fatal(err)
	}

	var typ string
	var valid bool
	err = s.db.QueryRowContext(t.Context(),
		"select typeof(data), json_valid(data, 8) from sboms where id = ?", res.ID).Scan(&typ, &valid)
	if err != nil {
		t.Fatal(err)
	}
	if typ != "blob" || !valid {
		t.Errorf("data should be valid JSONB blob, got typeof=%s valid=%v", typ, valid)
	}

	got, err := s.Document(t.Context(), res.ID)
	if err != nil {
		t.Fatal(err)
	}
	var gotVal, wantVal any
	if err := json.Unmarshal([]byte(got), &gotVal); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &wantVal); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotVal, wantVal) {
		t.Error("document read back from JSONB differs from the original")
	}

	if _, err := s.Document(t.Context(), 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestSearch(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)
	ingestFixtures(t, s)

	// key renders a match as "sbomID:name@version" for compact comparison.
	key := func(m Match) string { return fmt.Sprintf("%d:%s@%s", m.SBOMID, m.Component, m.Version) }
	tests := []struct {
		name   string
		filter Filter
		want   []string
	}{
		{
			"component across sboms", Filter{Component: "tomcat-catalina"},
			[]string{"1:tomcat-catalina@9.0.14", "3:tomcat-catalina@9.0.14"},
		},
		{
			"component is case-insensitive", Filter{Component: "acme application"},
			[]string{"1:Acme Application@9.1.1", "2:Acme Application@1.0.0"},
		},
		{
			"component and version", Filter{Component: "Acme Application", Version: "1.0.0"},
			[]string{"2:Acme Application@1.0.0"},
		},
		{"version only", Filter{Version: "9.0.14"}, []string{"1:tomcat-catalina@9.0.14", "3:tomcat-catalina@9.0.14"}},
		{
			"license from expression", Filter{License: "mit"},
			[]string{"4:hyper@0.14", "4:pretty_env_logger@0.4.0", "4:tokio@1.19.2"},
		},
		{
			"license id across formats", Filter{License: "Apache-2.0"},
			[]string{
				"1:tomcat-catalina@9.0.14", "3:tomcat-catalina@9.0.14",
				"4:hello-server-src@0.1.0", "4:pretty_env_logger@0.4.0",
			},
		},
		{"license with exception", Filter{License: "GPL-2.0"}, []string{"3:tomcat-catalina@9.0.14"}},
		{"license name", Filter{License: "some random license"}, []string{"1:myframework@1.0.0"}},
		{"license ref", Filter{License: "LicenseRef-MIT-Style-2"}, []string{"3:tomcat-catalina@9.0.14"}},
		{"placeholders are not licenses", Filter{License: "NOASSERTION"}, nil},
		{
			"all filters", Filter{Component: "tomcat-catalina", Version: "9.0.14", License: "EPL-2.0"},
			[]string{"3:tomcat-catalina@9.0.14"},
		},
		{"no match", Filter{Component: "hyper", License: "Apache-2.0"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			matches, err := s.Search(t.Context(), tt.filter)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, m := range matches {
				got = append(got, key(m))
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	if _, err := s.Search(t.Context(), Filter{}); !errors.Is(err, ErrEmptyFilter) {
		t.Errorf("empty filter: want ErrEmptyFilter, got %v", err)
	}
}

// TestSearchUsesIndexes guards against query changes that silently fall back
// to scanning every component.
func TestSearchUsesIndexes(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)
	ingestFixtures(t, s)

	filters := []Filter{
		{Component: "x"},
		{Version: "x"},
		{License: "x"},
		{Component: "x", Version: "x"},
		{Component: "x", License: "x"},
		{Version: "x", License: "x"},
		{Component: "x", Version: "x", License: "x"},
	}
	for _, f := range filters {
		plan := queryPlan(t, s, f)
		for _, step := range plan {
			if strings.HasPrefix(step, "SCAN ") {
				t.Errorf("filter %+v scans a table:\n%s", f, strings.Join(plan, "\n"))
				break
			}
		}
	}
}

func queryPlan(t *testing.T, s *Store, f Filter) []string {
	t.Helper()
	query, args, err := searchQuery(f)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.db.QueryContext(t.Context(), "explain query plan "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(plan) == 0 {
		t.Fatalf("empty query plan for %+v", f)
	}
	return plan
}
