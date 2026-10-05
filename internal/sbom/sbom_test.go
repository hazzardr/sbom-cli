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

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		fixture string
		want    Document
	}{
		{
			fixture: "cyclonedx-1.6-spec-valid-bom.json",
			want: Document{
				Format: FormatCycloneDX, SpecVersion: "1.6", Name: "Acme Application",
				DocumentID: "urn:uuid:3e671687-395b-41f5-a30f-a58921a69b79",
				// The pedigree ancestor (org.apache.tomcat/tomcat-catalina) is
				// not part of the BOM's inventory and must not be indexed.
				Components: []Component{
					{Name: "Acme Application", Version: "9.1.1", Type: "application"},
					{
						Name: "tomcat-catalina", Version: "9.0.14", Type: "application",
						PURL:     "pkg:maven/com.acme/tomcat-catalina@9.0.14?packaging=jar",
						Licenses: []string{"Apache-2.0"},
					},
					{
						Name: "mylibrary", Version: "1.0.0", Type: "library",
						PURL:     "pkg:maven/com.example/myapplication@1.0.0?packaging=war",
						Licenses: []string{"EPL-2.0 OR GPL-2.0-with-classpath-exception"},
					},
					{
						Name: "myframework", Version: "1.0.0", Type: "framework",
						PURL:     "pkg:maven/com.example/myframework@1.0.0?packaging=war",
						Licenses: []string{"Some random license"},
					},
				},
			},
		},
		{
			fixture: "cyclonedx-1.7-guide-bom-link.json",
			want: Document{
				Format: FormatCycloneDX, SpecVersion: "1.7",
				DocumentID: "urn:uuid:3e671687-395b-41f5-a30f-a58921a69b79",
				Components: []Component{
					{Name: "Acme Application", Version: "1.0.0", Type: "application"},
					{Name: "Acme Threat Model", Type: "data"},
				},
			},
		},
		{
			fixture: "cyclonedx-1.7-spec-license-choice.json",
			want: Document{
				Format: FormatCycloneDX, SpecVersion: "1.7",
				DocumentID: "urn:uuid:b1ef52c6-7cd8-43d5-9e42-5e69044bbe9e",
				Components: []Component{{
					Name: "tomcat-catalina", Version: "9.0.14", Type: "application",
					Licenses: []string{
						"Apache-2.0", "EPL-2.0 OR GPL-2.0 WITH Classpath-exception-2.0",
						"My Own License", "LicenseRef-MIT-Style-2",
					},
				}},
			},
		},
		{
			fixture: "spdx-3.0.1-examples-example11.json",
			want: Document{
				Format: FormatSPDX, SpecVersion: "3.0.1", Name: "SBOM-SPDX-2d85f548-12fa-46d5-87ce-5e78e5e111f4",
				DocumentID: "https://spdx.org/spdxdocs/k8s-releng-bom-7c6a33ab-bd76-4b06-b291-a850e0815b07-specv3/document0",
				Components: []Component{
					{
						Name: "hello-server-src", Version: "0.1.0", Licenses: []string{"Apache-2.0"},
						PURL: "pkg:deb/debian/libselinux1-dev@3.1-3?arch=s390x",
					},
					{
						Name: "hyper", Version: "0.14", PURL: "pkg:cargo/hyper@0.14",
						Licenses: []string{"MIT", "NOASSERTION"},
					},
					{
						Name: "tokio", Version: "1.19.2", PURL: "pkg:cargo/tokio@1.19.2",
						Licenses: []string{"MIT", "NOASSERTION"},
					},
					{
						Name: "pretty_env_logger", Version: "0.4.0", PURL: "pkg:cargo/pretty_env_logger@0.4.0",
						Licenses: []string{"(MIT OR Apache-2.0)", "NOASSERTION"},
					},
				},
			},
		},
		{
			fixture: "spdx-3.0.1-spec-package-sbom.json",
			want: Document{
				Format: FormatSPDX, SpecVersion: "3.0.1", DocumentID: "http://spdx.example.com/Document1",
				Components: []Component{{Name: "my-package", Version: "1.0"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			t.Parallel()
			doc, err := Parse(readFixture(t, tt.fixture))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*doc, tt.want) {
				t.Errorf("\n got %+v\nwant %+v", *doc, tt.want)
			}
		})
	}
}

func TestParseUnsupported(t *testing.T) {
	t.Parallel()
	fixtures := []string{
		"unsupported/cyclonedx-1.4-examples-laravel.json",
		"unsupported/spdx-2.3-examples-minimal-sbom.json",
	}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse(readFixture(t, name)); !errors.Is(err, ErrUnsupportedFormat) {
				t.Errorf("want ErrUnsupportedFormat, got %v", err)
			}
		})
	}

	// Minimal hand-written inputs, kept on purpose: they need no real document.
	// "spdx 3.1" is the only test of the SPDX version check (spdx.go); the
	// SPDX 2.3 fixture is rejected earlier, by format detection, so it never
	// reaches that check. No published SPDX 3.1 document exists to use instead.
	inline := map[string]string{
		"spdx 3.1":    `{"@context": "x", "@graph": [{"type": "CreationInfo", "specVersion": "3.1.0"}]}`,
		"not an sbom": `{"hello": "world"}`,
	}
	for name, input := range inline {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse([]byte(input)); !errors.Is(err, ErrUnsupportedFormat) {
				t.Errorf("want ErrUnsupportedFormat, got %v", err)
			}
		})
	}
}

func TestParseMalformed(t *testing.T) {
	t.Parallel()
	inputs := map[string]string{
		"invalid json":           `{"bomFormat": "CycloneDX",`,
		"cyclonedx wrong shape":  `{"bomFormat": "CycloneDX", "specVersion": "1.6", "components": {}}`,
		"spdx graph wrong shape": `{"@context": "x", "@graph": {}}`,
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse([]byte(input)); !errors.Is(err, ErrMalformed) {
				t.Errorf("want ErrMalformed, got %v", err)
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
		{[]string{"Some random license"}, []string{"Some random license"}},
		{[]string{"LicenseRef-MIT-Style-2"}, []string{"LicenseRef-MIT-Style-2"}},
	}
	for _, tt := range tests {
		if got := LicenseIDs(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("LicenseIDs(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
