package sbom

import (
	"errors"
	"os"
	"reflect"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseCycloneDX(t *testing.T) {
	t.Parallel()
	doc, err := Parse(readFixture(t, "cyclonedx-1.6.json"))
	if err != nil {
		t.Fatal(err)
	}

	if doc.Format != FormatCycloneDX || doc.SpecVersion != "1.6" || doc.Name != "acme-web" ||
		doc.DocumentID != "urn:uuid:3e671687-395b-41f5-a30f-a58921a69b79" {
		t.Errorf("unexpected document header: %+v", doc)
	}
	want := []Component{
		{Name: "acme-web", Version: "2.4.0", Type: "application", Licenses: []string{"Apache-2.0"}},
		{
			Name: "log4j-core", Version: "2.14.1", Type: "library", Licenses: []string{"Apache-2.0"},
			PURL: "pkg:maven/org.apache.logging.log4j/log4j-core@2.14.1",
		},
		{
			Name: "log4j-api", Version: "2.14.1", Type: "library", Licenses: []string{"Apache-2.0"},
			PURL: "pkg:maven/org.apache.logging.log4j/log4j-api@2.14.1",
		},
		{
			Name: "serde", Version: "1.0.210", Type: "library", Licenses: []string{"MIT OR Apache-2.0"},
			PURL: "pkg:cargo/serde@1.0.210",
		},
		{Name: "internal-utils", Version: "0.3.0", Type: "library", Licenses: []string{"Acme Proprietary License"}},
	}
	if !reflect.DeepEqual(doc.Components, want) {
		t.Errorf("components:\n got %+v\nwant %+v", doc.Components, want)
	}
}

func TestParseSPDX(t *testing.T) {
	t.Parallel()
	doc, err := Parse(readFixture(t, "spdx-3.0.1.json"))
	if err != nil {
		t.Fatal(err)
	}

	if doc.Format != FormatSPDX || doc.SpecVersion != "3.0.1" || doc.Name != "acme-cli" ||
		doc.DocumentID != "https://example.com/acme-cli/document" {
		t.Errorf("unexpected document header: %+v", doc)
	}
	want := []Component{
		{
			Name: "acme-cli", Version: "1.0.0", Type: "application",
			Licenses: []string{"GPL-2.0-only WITH Classpath-exception-2.0", "NOASSERTION"},
		},
		{Name: "hyper", Version: "0.14.28", PURL: "pkg:cargo/hyper@0.14.28", Type: "library", Licenses: []string{"MIT"}},
		{
			Name: "log4j-core", Version: "2.17.1", Licenses: []string{"Apache-2.0"},
			PURL: "pkg:maven/org.apache.logging.log4j/log4j-core@2.17.1",
		},
	}
	if !reflect.DeepEqual(doc.Components, want) {
		t.Errorf("components:\n got %+v\nwant %+v", doc.Components, want)
	}
}

func TestParseUnsupported(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"cyclonedx 1.5": `{"bomFormat": "CycloneDX", "specVersion": "1.5"}`,
		"spdx 2.3":      `{"spdxVersion": "SPDX-2.3", "packages": []}`,
		"spdx 3.1":      `{"@context": "x", "@graph": [{"type": "CreationInfo", "specVersion": "3.1.0"}]}`,
		"not an sbom":   `{"hello": "world"}`,
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse([]byte(input)); !errors.Is(err, ErrUnsupportedFormat) {
				t.Errorf("want ErrUnsupportedFormat, got %v", err)
			}
		})
	}
}

func TestLicenseIDs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   []string
		want []string
	}{
		{[]string{"MIT"}, []string{"MIT"}},
		{[]string{"MIT OR Apache-2.0"}, []string{"MIT", "Apache-2.0"}},
		{[]string{"(MIT OR Apache-2.0) AND BSD-3-Clause"}, []string{"MIT", "Apache-2.0", "BSD-3-Clause"}},
		{[]string{"GPL-2.0-only WITH Classpath-exception-2.0"}, []string{"GPL-2.0-only"}},
		{[]string{"Apache-2.0", "apache-2.0", "MIT"}, []string{"Apache-2.0", "MIT"}},
		{[]string{"NOASSERTION", "NONE", ""}, nil},
		{[]string{"Acme Proprietary License"}, []string{"Acme Proprietary License"}},
	}
	for _, tt := range tests {
		if got := LicenseIDs(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("LicenseIDs(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
