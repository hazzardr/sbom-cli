// Command generate writes synthetic SBOMs for the k6 performance tests.
//
// It alternates CycloneDX 1.6 and SPDX 3.0.1 documents shaped like the real
// fixtures in internal/sbom/testdata, and writes manifest.json listing the
// files plus sample component names, versions, and licenses for queries.
// Output is deterministic, so runs with the same flags are comparable.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type manifest struct {
	Count      int         `json:"count"`
	Components int         `json:"components"`
	Names      int         `json:"names"`
	Files      []string    `json:"files"`
	Samples    []component `json:"samples"`
	Licenses   []string    `json:"licenses"`
}

type component struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	PURL    string `json:"purl"`
	License string `json:"license"`
}

const commercialLicense = "Example Commercial License"

// licensePool assigns licenses to packages (index = package % len), weighted
// toward common licenses the way real dependency trees are.
var licensePool = weighted([]struct {
	license string
	weight  int
}{
	{"MIT", 6}, {"Apache-2.0", 4}, {"MIT OR Apache-2.0", 2},
	{"BSD-3-Clause", 1}, {"ISC", 1}, {"BSD-2-Clause", 1}, {"MPL-2.0", 1},
	{"GPL-2.0-only WITH Classpath-exception-2.0", 1}, {"LGPL-2.1-or-later", 1},
	{"Apache-2.0 AND BSD-3-Clause", 1}, {commercialLicense, 1},
})

func weighted(entries []struct {
	license string
	weight  int
},
) []string {
	var pool []string
	for _, e := range entries {
		for range e.weight {
			pool = append(pool, e.license)
		}
	}
	return pool
}

// queryLicenses are the license IDs queries can match (expressions split).
var queryLicenses = []string{
	"MIT", "Apache-2.0", "BSD-3-Clause", "ISC", "BSD-2-Clause", "MPL-2.0",
	"GPL-2.0-only", "LGPL-2.1-or-later", commercialLicense, "LicenseRef-Example-Commercial",
}

const sampleCount = 500

func main() {
	out := flag.String("out", "data", "output directory")
	count := flag.Int("count", 100, "number of SBOMs")
	components := flag.Int("components", 500, "components per SBOM")
	names := flag.Int("names", 5000, "distinct package names to draw from (lower = more overlap between SBOMs)")
	force := flag.Bool("force", false, "regenerate even if the manifest matches")
	flag.Parse()

	if err := run(*out, *count, *components, *names, *force); err != nil {
		log.Fatal(err)
	}
}

func run(out string, count, components, names int, force bool) error {
	manifestPath := filepath.Join(out, "manifest.json")
	if !force && upToDate(manifestPath, count, components, names) {
		fmt.Printf("%s: up to date (%d SBOMs x %d components)\n", out, count, components)
		return nil
	}
	if err := os.MkdirAll(out, 0o750); err != nil {
		return err
	}

	m := manifest{Count: count, Components: components, Names: names, Licenses: queryLicenses}
	for d := range count {
		comps := make([]component, components)
		for i := range comps {
			comps[i] = newComponent(d, i, names)
		}
		var name string
		var doc any
		if d%2 == 0 {
			name, doc = fmt.Sprintf("sbom-%05d.cdx.json", d), cycloneDX(d, comps)
		} else {
			name, doc = fmt.Sprintf("sbom-%05d.spdx.json", d), spdx(d, comps)
		}
		if err := writeJSON(filepath.Join(out, name), doc); err != nil {
			return err
		}
		m.Files = append(m.Files, name)
		if len(m.Samples) < sampleCount {
			m.Samples = append(m.Samples, comps[d%components])
		}
	}
	if err := writeJSON(manifestPath, m); err != nil {
		return err
	}
	fmt.Printf("%s: wrote %d SBOMs x %d components\n", out, count, components)
	return nil
}

func upToDate(path string, count, components, names int) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var m manifest
	if json.Unmarshal(data, &m) != nil {
		return false
	}
	return m.Count == count && m.Components == components && m.Names == names
}

// newComponent derives component i of SBOM d. Packages are drawn from a fixed
// pool so the same name@version recurs across SBOMs, as in real fleets.
func newComponent(d, i, names int) component {
	p := (d*7919 + i*104729) % names
	name := fmt.Sprintf("pkg-%05d", p)
	// SBOMs in the same "release window" of 10 share versions.
	version := fmt.Sprintf("%d.%d.%d", p%5, (p+d/10)%20, p%7)
	var purl string
	switch p % 4 {
	case 0:
		purl = fmt.Sprintf("pkg:npm/%s@%s", name, version)
	case 1:
		purl = fmt.Sprintf("pkg:pypi/%s@%s", name, version)
	case 2:
		purl = fmt.Sprintf("pkg:maven/com.example/%s@%s", name, version)
	default:
		purl = fmt.Sprintf("pkg:golang/example.com/%s@v%s", name, version)
	}
	return component{Name: name, Version: version, PURL: purl, License: licensePool[p%len(licensePool)]}
}

type cdxBOM struct {
	BOMFormat    string         `json:"bomFormat"`
	SpecVersion  string         `json:"specVersion"`
	SerialNumber string         `json:"serialNumber"`
	Version      int            `json:"version"`
	Metadata     cdxMetadata    `json:"metadata"`
	Components   []cdxComponent `json:"components"`
}

type cdxMetadata struct {
	Timestamp string       `json:"timestamp"`
	Component cdxComponent `json:"component"`
}

type cdxComponent struct {
	Type        string       `json:"type"`
	BOMRef      string       `json:"bom-ref,omitempty"`
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	PURL        string       `json:"purl,omitempty"`
	Description string       `json:"description,omitempty"`
	Hashes      []cdxHash    `json:"hashes,omitempty"`
	Licenses    []cdxLicense `json:"licenses,omitempty"`
}

type cdxHash struct {
	Alg     string `json:"alg"`
	Content string `json:"content"`
}

type cdxLicense struct {
	License    *cdxLicenseRef `json:"license,omitempty"`
	Expression string         `json:"expression,omitempty"`
}

type cdxLicenseRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

func cycloneDX(d int, comps []component) cdxBOM {
	items := make([]cdxComponent, len(comps))
	for i, c := range comps {
		items[i] = cdxComponent{
			Type:        "library",
			BOMRef:      c.PURL,
			Name:        c.Name,
			Version:     c.Version,
			PURL:        c.PURL,
			Description: "Synthetic package generated for performance tests.",
			Hashes:      []cdxHash{{Alg: "SHA-256", Content: fmt.Sprintf("%064x", d*len(comps)+i)}},
			Licenses:    []cdxLicense{newCDXLicense(c.License)},
		}
	}
	return cdxBOM{
		BOMFormat:    "CycloneDX",
		SpecVersion:  "1.6",
		SerialNumber: fmt.Sprintf("urn:uuid:00000000-0000-4000-8000-%012d", d),
		Version:      1,
		Metadata: cdxMetadata{
			Timestamp: "2026-10-05T00:00:00Z",
			Component: cdxComponent{Type: "application", Name: fmt.Sprintf("app-%05d", d), Version: "1.0.0"},
		},
		Components: items,
	}
}

func newCDXLicense(license string) cdxLicense {
	switch license {
	case commercialLicense:
		return cdxLicense{License: &cdxLicenseRef{Name: license}}
	case "MIT OR Apache-2.0", "Apache-2.0 AND BSD-3-Clause", "GPL-2.0-only WITH Classpath-exception-2.0":
		return cdxLicense{Expression: license}
	default:
		return cdxLicense{License: &cdxLicenseRef{ID: license}}
	}
}

type spdxDocument struct {
	Context string        `json:"@context"`
	Graph   []spdxElement `json:"@graph"`
}

// spdxElement holds the union of the element properties written here.
type spdxElement struct {
	Type             string   `json:"type"`
	ID               string   `json:"@id,omitempty"`
	SpdxID           string   `json:"spdxId,omitempty"`
	CreationInfo     string   `json:"creationInfo,omitempty"`
	SpecVersion      string   `json:"specVersion,omitempty"`
	Created          string   `json:"created,omitempty"`
	CreatedBy        []string `json:"createdBy,omitempty"`
	Name             string   `json:"name,omitempty"`
	RootElement      []string `json:"rootElement,omitempty"`
	PackageVersion   string   `json:"software_packageVersion,omitempty"`
	PackageURL       string   `json:"software_packageUrl,omitempty"`
	PrimaryPurpose   string   `json:"software_primaryPurpose,omitempty"`
	LicenseExpr      string   `json:"simplelicensing_licenseExpression,omitempty"`
	RelationshipType string   `json:"relationshipType,omitempty"`
	From             string   `json:"from,omitempty"`
	To               []string `json:"to,omitempty"`
}

func spdx(d int, comps []component) spdxDocument {
	base := fmt.Sprintf("https://example.com/perf/sbom-%05d", d)
	const creationInfo = "_:creationinfo"
	graph := []spdxElement{
		{
			Type: "CreationInfo", ID: creationInfo, SpecVersion: "3.0.1",
			Created: "2026-10-05T00:00:00Z", CreatedBy: []string{base + "/agent"},
		},
		{
			Type: "SpdxDocument", SpdxID: base + "/document", Name: fmt.Sprintf("app-%05d", d),
			CreationInfo: creationInfo, RootElement: []string{base + "/package/0"},
		},
	}
	licenseIDs := map[string]string{}
	for i, c := range comps {
		expr := c.License
		if expr == commercialLicense {
			// Free-text names aren't valid SPDX expressions; SPDX uses LicenseRef-.
			expr = "LicenseRef-Example-Commercial"
		}
		pkgID := fmt.Sprintf("%s/package/%d", base, i)
		graph = append(graph, spdxElement{
			Type: "software_Package", SpdxID: pkgID, CreationInfo: creationInfo,
			Name: c.Name, PackageVersion: c.Version, PackageURL: c.PURL, PrimaryPurpose: "library",
		})
		licID, ok := licenseIDs[expr]
		if !ok {
			licID = fmt.Sprintf("%s/license/%d", base, len(licenseIDs))
			licenseIDs[expr] = licID
			graph = append(graph, spdxElement{
				Type: "simplelicensing_LicenseExpression", SpdxID: licID, CreationInfo: creationInfo,
				LicenseExpr: expr,
			})
		}
		graph = append(graph, spdxElement{
			Type: "Relationship", SpdxID: fmt.Sprintf("%s/relationship/%d", base, i), CreationInfo: creationInfo,
			RelationshipType: "hasDeclaredLicense", From: pkgID, To: []string{licID},
		})
	}
	return spdxDocument{Context: "https://spdx.org/rdf/3.0.1/spdx-context.jsonld", Graph: graph}
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
