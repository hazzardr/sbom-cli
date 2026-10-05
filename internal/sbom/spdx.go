package sbom

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	spdxSpecVersionPrefix = "3.0"
	spdxListedLicenseIRI  = "https://spdx.org/licenses/"
)

// spdxDocument is an SPDX 3 JSON-LD serialization: a flat @graph of elements
// that reference each other by spdxId.
type spdxDocument struct {
	Graph []spdxElement `json:"@graph"`
}

// spdxElement holds the union of the element properties this package reads.
type spdxElement struct {
	Type             string   `json:"type"`
	SpdxID           string   `json:"spdxId"`
	Name             string   `json:"name"`
	SpecVersion      string   `json:"specVersion"`
	PackageVersion   string   `json:"software_packageVersion"`
	PackageURL       string   `json:"software_packageUrl"`
	PrimaryPurpose   string   `json:"software_primaryPurpose"`
	RelationshipType string   `json:"relationshipType"`
	From             string   `json:"from"`
	To               []string `json:"to"`
	LicenseExpr      string   `json:"simplelicensing_licenseExpression"`
}

func parseSPDX(data []byte) (*Document, error) {
	var raw spdxDocument
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: decode SPDX: %w", ErrMalformed, err)
	}

	doc := &Document{Format: FormatSPDX}
	byID := make(map[string]*spdxElement, len(raw.Graph))
	for i := range raw.Graph {
		el := &raw.Graph[i]
		if el.SpdxID != "" {
			byID[el.SpdxID] = el
		}
		switch el.Type {
		case "CreationInfo":
			if doc.SpecVersion == "" {
				doc.SpecVersion = el.SpecVersion
			}
		case "SpdxDocument":
			doc.DocumentID = el.SpdxID
			doc.Name = el.Name
		}
	}
	if !strings.HasPrefix(doc.SpecVersion, spdxSpecVersionPrefix) {
		return nil, fmt.Errorf("%w: SPDX specVersion %q (want %s.x)",
			ErrUnsupportedFormat, doc.SpecVersion, spdxSpecVersionPrefix)
	}

	licenses := spdxPackageLicenses(raw.Graph, byID)
	for i := range raw.Graph {
		el := &raw.Graph[i]
		if el.Type != "software_Package" {
			continue
		}
		doc.Components = append(doc.Components, Component{
			Name:     el.Name,
			Version:  el.PackageVersion,
			PURL:     el.PackageURL,
			Type:     el.PrimaryPurpose,
			Licenses: licenses[el.SpdxID],
		})
	}
	return doc, nil
}

// spdxPackageLicenses resolves hasDeclaredLicense and hasConcludedLicense
// relationships into license expressions keyed by the "from" element.
func spdxPackageLicenses(graph []spdxElement, byID map[string]*spdxElement) map[string][]string {
	out := make(map[string][]string)
	for i := range graph {
		rel := &graph[i]
		if rel.RelationshipType != "hasDeclaredLicense" && rel.RelationshipType != "hasConcludedLicense" {
			continue
		}
		for _, to := range rel.To {
			expr := spdxLicenseExpression(to, byID)
			if expr != "" && !slices.Contains(out[rel.From], expr) {
				out[rel.From] = append(out[rel.From], expr)
			}
		}
	}
	return out
}

func spdxLicenseExpression(id string, byID map[string]*spdxElement) string {
	el, inGraph := byID[id]
	if inGraph && el.LicenseExpr != "" {
		return el.LicenseExpr
	}
	// Listed licenses are identified by IRI and may be referenced without
	// being serialized in the graph.
	if name, ok := strings.CutPrefix(id, spdxListedLicenseIRI); ok {
		return name
	}
	if inGraph && strings.HasPrefix(el.Type, "expandedlicensing_") {
		return el.Name
	}
	return ""
}
