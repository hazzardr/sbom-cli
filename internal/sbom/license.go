package sbom

import "strings"

// LicenseIDs splits SPDX license expressions into the individual license
// identifiers they reference, so "MIT OR Apache-2.0" is findable by either
// ID. Operators, WITH exceptions, and the NONE/NOASSERTION placeholders are
// dropped. Values that are not expressions, such as free-text CycloneDX
// license names, are returned whole.
func LicenseIDs(exprs []string) []string {
	var ids []string
	seen := make(map[string]bool)
	add := func(id string) {
		key := strings.ToLower(id)
		if id == "" || seen[key] || key == "none" || key == "noassertion" {
			return
		}
		seen[key] = true
		ids = append(ids, id)
	}

	for _, expr := range exprs {
		expr = strings.TrimSpace(expr)
		if !isExpression(expr) {
			add(expr)
			continue
		}
		tokens := strings.Fields(strings.NewReplacer("(", " ", ")", " ").Replace(expr))
		for i := 0; i < len(tokens); i++ {
			switch strings.ToUpper(tokens[i]) {
			case "AND", "OR":
			case "WITH":
				i++ // skip the exception identifier
			default:
				add(tokens[i])
			}
		}
	}
	return ids
}

// isExpression reports whether s looks like an SPDX license expression
// rather than a free-text license name: either a single token or a
// combination joined by AND/OR/WITH operators.
func isExpression(s string) bool {
	fields := strings.Fields(strings.NewReplacer("(", " ", ")", " ").Replace(s))
	if len(fields) <= 1 {
		return true
	}
	for i, f := range fields {
		isOp := f == "AND" || f == "OR" || f == "WITH" ||
			f == "and" || f == "or" || f == "with"
		if (i%2 == 1) != isOp {
			return false
		}
	}
	return len(fields)%2 == 1
}
