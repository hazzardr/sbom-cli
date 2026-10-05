// Package sbom parses CycloneDX 1.6 and SPDX 3.0 JSON documents into a
// format-neutral list of components.
package sbom

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Format identifies the SBOM specification a document was written in.
type Format string

const (
	FormatCycloneDX Format = "cyclonedx"
	FormatSPDX      Format = "spdx"
)

// ErrUnsupportedFormat is returned when a document is not CycloneDX 1.6 or
// SPDX 3.0 JSON.
var ErrUnsupportedFormat = errors.New("unsupported SBOM format")

// Document is the format-neutral view of an SBOM used for indexing.
type Document struct {
	Format      Format
	SpecVersion string
	// DocumentID is the CycloneDX serialNumber or the SPDX document spdxId.
	DocumentID string
	Name       string
	Components []Component
}

// Component is a single piece of software listed in an SBOM.
type Component struct {
	Name    string
	Version string
	PURL    string
	Type    string
	// Licenses holds the license expressions or names as written in the
	// document. Use LicenseIDs to split them into searchable identifiers.
	Licenses []string
}

// Parse detects the format of a JSON SBOM and extracts its components.
func Parse(data []byte) (*Document, error) {
	var probe struct {
		BOMFormat string          `json:"bomFormat"`
		Context   json.RawMessage `json:"@context"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	switch {
	case probe.BOMFormat == "CycloneDX":
		return parseCycloneDX(data)
	case len(probe.Context) > 0:
		return parseSPDX(data)
	default:
		return nil, fmt.Errorf("%w: expected CycloneDX or SPDX 3 JSON-LD", ErrUnsupportedFormat)
	}
}
