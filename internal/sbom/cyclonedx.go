package sbom

import (
	"encoding/json"
	"fmt"
)

const cycloneDXSpecVersion = "1.6"

type cdxBOM struct {
	SpecVersion  string `json:"specVersion"`
	SerialNumber string `json:"serialNumber"`
	Metadata     struct {
		Component *cdxComponent `json:"component"`
	} `json:"metadata"`
	Components []cdxComponent `json:"components"`
}

type cdxComponent struct {
	Type       string         `json:"type"`
	Name       string         `json:"name"`
	Version    string         `json:"version"`
	PURL       string         `json:"purl"`
	Licenses   []cdxLicense   `json:"licenses"`
	Components []cdxComponent `json:"components"`
}

// cdxLicense is one entry of a CycloneDX licenses array: either a single
// license (by SPDX id or free-text name) or an SPDX license expression.
type cdxLicense struct {
	License *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"license"`
	Expression string `json:"expression"`
}

func parseCycloneDX(data []byte) (*Document, error) {
	var bom cdxBOM
	if err := json.Unmarshal(data, &bom); err != nil {
		return nil, fmt.Errorf("decode CycloneDX: %w", err)
	}
	if bom.SpecVersion != cycloneDXSpecVersion {
		return nil, fmt.Errorf("%w: CycloneDX specVersion %q (want %s)",
			ErrUnsupportedFormat, bom.SpecVersion, cycloneDXSpecVersion)
	}

	doc := &Document{
		Format:      FormatCycloneDX,
		SpecVersion: bom.SpecVersion,
		DocumentID:  bom.SerialNumber,
	}
	// The metadata component is the subject of the SBOM; index it alongside
	// its dependencies so it is queryable the same way as in SPDX, where the
	// root package is an ordinary element.
	if root := bom.Metadata.Component; root != nil {
		doc.Name = root.Name
		doc.Components = appendCDXComponents(doc.Components, []cdxComponent{*root})
	}
	doc.Components = appendCDXComponents(doc.Components, bom.Components)
	return doc, nil
}

// appendCDXComponents flattens nested components depth-first.
func appendCDXComponents(dst []Component, src []cdxComponent) []Component {
	for _, c := range src {
		dst = append(dst, Component{
			Name:     c.Name,
			Version:  c.Version,
			PURL:     c.PURL,
			Type:     c.Type,
			Licenses: cdxLicenses(c.Licenses),
		})
		dst = appendCDXComponents(dst, c.Components)
	}
	return dst
}

func cdxLicenses(entries []cdxLicense) []string {
	var out []string
	for _, l := range entries {
		switch {
		case l.Expression != "":
			out = append(out, l.Expression)
		case l.License != nil && l.License.ID != "":
			out = append(out, l.License.ID)
		case l.License != nil && l.License.Name != "":
			out = append(out, l.License.Name)
		}
	}
	return out
}
