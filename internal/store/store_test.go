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

// ingestFixtures loads the CycloneDX fixture as SBOM 1 and SPDX as SBOM 2.
func ingestFixtures(t *testing.T, s *Store) {
	t.Helper()
	for _, name := range []string{"cyclonedx-1.6.json", "spdx-3.0.1.json"} {
		if _, err := s.Ingest(t.Context(), name, readFixture(t, name)); err != nil {
			t.Fatalf("ingest %s: %v", name, err)
		}
	}
}

func TestIngestDeduplicates(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)
	raw := readFixture(t, "cyclonedx-1.6.json")

	first, err := s.Ingest(t.Context(), "a.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	if first.Duplicate || first.Components != 5 {
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
	if len(list) != 1 || list[0].ComponentCount != 5 || list[0].Name != "acme-web" {
		t.Errorf("list: %+v", list)
	}
}

func TestIngestRejectsUnsupported(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)
	if _, err := s.Ingest(t.Context(), "x.json", []byte(`{"bomFormat":"CycloneDX","specVersion":"1.4"}`)); err == nil {
		t.Fatal("expected error for CycloneDX 1.4")
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
	raw := readFixture(t, "spdx-3.0.1.json")
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
		{"component across formats", Filter{Component: "log4j-core"}, []string{"1:log4j-core@2.14.1", "2:log4j-core@2.17.1"}},
		{
			"component is case-insensitive", Filter{Component: "LOG4J-CORE"},
			[]string{"1:log4j-core@2.14.1", "2:log4j-core@2.17.1"},
		},
		{"component and version", Filter{Component: "log4j-core", Version: "2.14.1"}, []string{"1:log4j-core@2.14.1"}},
		{"version only", Filter{Version: "2.14.1"}, []string{"1:log4j-api@2.14.1", "1:log4j-core@2.14.1"}},
		{"license from expression", Filter{License: "mit"}, []string{"1:serde@1.0.210", "2:hyper@0.14.28"}},
		{
			"license id", Filter{License: "Apache-2.0"},
			[]string{
				"1:acme-web@2.4.0", "1:log4j-api@2.14.1", "1:log4j-core@2.14.1",
				"1:serde@1.0.210", "2:log4j-core@2.17.1",
			},
		},
		{"license with exception", Filter{License: "GPL-2.0-only"}, []string{"2:acme-cli@1.0.0"}},
		{"license name", Filter{License: "Acme Proprietary License"}, []string{"1:internal-utils@0.3.0"}},
		{"placeholders are not licenses", Filter{License: "NOASSERTION"}, nil},
		{"all filters", Filter{Component: "serde", Version: "1.0.210", License: "MIT"}, []string{"1:serde@1.0.210"}},
		{"no match", Filter{Component: "log4j-core", License: "MIT"}, nil},
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
