package store

import (
	"bytes"
	"encoding/json"
	"slices"
	"sync"
	"testing"

	"github.com/hazzardr/sbom-cli/internal/sbom"
)

const dedupeFixture = "cyclonedx-1.6-spec-valid-bom.json"

// reencode decodes raw and re-encodes it after applying edit, which yields
// sorted object keys and compact output.
func reencode(t *testing.T, raw []byte, edit func(doc map[string]any)) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if edit != nil {
		edit(doc)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// escapedA is the JSON escape sequence for "A" (backslash, u, 0041), spelled
// out as bytes so it cannot be mistaken for, or turned into, a literal "A".
var escapedA = []byte{0x5c, 'u', '0', '0', '4', '1'}

func TestIngestDetectsReformattedDuplicates(t *testing.T) {
	t.Parallel()
	original := readFixture(t, dedupeFixture)
	var compact, tabbed bytes.Buffer
	if err := json.Compact(&compact, original); err != nil {
		t.Fatal(err)
	}
	if err := json.Indent(&tabbed, compact.Bytes(), "", "\t"); err != nil {
		t.Fatal(err)
	}

	variants := map[string][]byte{
		"minified":           compact.Bytes(),
		"tab indented":       tabbed.Bytes(),
		"CRLF line endings":  bytes.ReplaceAll(original, []byte("\n"), []byte("\r\n")),
		"trailing newlines":  append(slices.Clone(original), "\n\n"...),
		"object keys sorted": reencode(t, original, nil),
		"unicode escape": bytes.Replace(original, []byte(`"Acme Application"`),
			slices.Concat([]byte(`"`), escapedA, []byte(`cme Application"`)), 1),
	}
	for name, variant := range variants {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if bytes.Equal(variant, original) {
				t.Fatal("variant is byte-identical to the original; test is not meaningful")
			}
			s := openTestStore(t)
			first, err := s.Ingest(t.Context(), "original.json", original)
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.Ingest(t.Context(), "variant.json", variant)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Duplicate || got.ID != first.ID {
				t.Errorf("want duplicate of SBOM %d, got %+v", first.ID, got)
			}
		})
	}
}

func TestIngestKeepsDistinctDocuments(t *testing.T) {
	t.Parallel()
	original := readFixture(t, dedupeFixture)

	variants := map[string][]byte{
		"value changed": bytes.Replace(original, []byte(`"9.1.1"`), []byte(`"9.1.2"`), 1),
		// Number literals are not normalized; see contentDigest.
		"number literal": bytes.Replace(original, []byte(`"version": 1,`), []byte(`"version": 1.0,`), 1),
		"array reordered": reencode(t, original, func(doc map[string]any) {
			slices.Reverse(doc["components"].([]any))
		}),
	}
	for name, variant := range variants {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if bytes.Equal(variant, original) {
				t.Fatal("variant is byte-identical to the original; test is not meaningful")
			}
			s := openTestStore(t)
			if _, err := s.Ingest(t.Context(), "original.json", original); err != nil {
				t.Fatal(err)
			}
			got, err := s.Ingest(t.Context(), "variant.json", variant)
			if err != nil {
				t.Fatal(err)
			}
			if got.Duplicate {
				t.Errorf("want a new SBOM, got duplicate %+v", got)
			}
		})
	}
}

// TestInsertLosingRaceReportsDuplicate simulates an ingest whose duplicate
// check ran before a concurrent ingest committed the same document.
func TestInsertLosingRaceReportsDuplicate(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)
	raw := readFixture(t, dedupeFixture)
	doc, err := sbom.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := contentDigest(raw)
	if err != nil {
		t.Fatal(err)
	}

	first, err := s.insert(t.Context(), IngestResult{}, doc, "a.json", digest, raw)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.insert(t.Context(), IngestResult{}, doc, "b.json", digest, raw)
	if err != nil {
		t.Fatalf("losing insert should report a duplicate, got error: %v", err)
	}
	if !second.Duplicate || second.ID != first.ID {
		t.Errorf("want duplicate of SBOM %d, got %+v", first.ID, second)
	}

	var components int
	if err := s.db.QueryRowContext(t.Context(), "select count(*) from components").Scan(&components); err != nil {
		t.Fatal(err)
	}
	if components != len(doc.Components) {
		t.Errorf("losing insert should be rolled back: %d components stored, want %d", components, len(doc.Components))
	}
}

func TestConcurrentIngestStoresOnce(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)
	raw := readFixture(t, dedupeFixture)

	const workers = 8
	results := make([]IngestResult, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Go(func() {
			results[i], errs[i] = s.Ingest(t.Context(), "concurrent.json", raw)
		})
	}
	wg.Wait()

	stored := 0
	for i, res := range results {
		if errs[i] != nil {
			t.Fatalf("worker %d: %v", i, errs[i])
		}
		if !res.Duplicate {
			stored++
		}
		if res.ID != results[0].ID {
			t.Errorf("worker %d got SBOM %d, want %d", i, res.ID, results[0].ID)
		}
	}
	if stored != 1 {
		t.Errorf("want exactly one non-duplicate result, got %d", stored)
	}
}
