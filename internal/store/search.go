package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrEmptyFilter is returned when a search specifies no criteria.
var ErrEmptyFilter = errors.New("at least one of component, version, or license is required")

// Filter selects components. Empty fields are ignored; set fields must all
// match. Component and License match case-insensitively; Version is exact.
type Filter struct {
	Component string
	Version   string
	License   string
}

// Match is a component that satisfied a Filter, with its parent SBOM.
type Match struct {
	SBOMID    int64  `json:"sbom_id"`
	SBOMName  string `json:"sbom_name"`
	Component string `json:"component"`
	Version   string `json:"version"`
	PURL      string `json:"purl,omitempty"`
	Type      string `json:"type,omitempty"`
	Licenses  string `json:"licenses,omitempty"`
}

// Search returns the components matching f across all stored SBOMs.
func (s *Store) Search(ctx context.Context, f Filter) ([]Match, error) {
	query, args, err := searchQuery(f)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var matches []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.SBOMID, &m.SBOMName, &m.Component, &m.Version, &m.PURL, &m.Type, &m.Licenses); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return matches, nil
}

// searchQuery builds the search SQL with only the predicates in use. This is
// hand-written rather than a sqlc query because the "? IS NULL OR col = ?"
// pattern for optional filters stops SQLite from using the indexes.
func searchQuery(f Filter) (string, []any, error) {
	var where []string
	var args []any
	if f.Component != "" {
		where = append(where, "c.name = ?")
		args = append(args, f.Component)
	}
	if f.Version != "" {
		where = append(where, "c.version = ?")
		args = append(args, f.Version)
	}
	if f.License != "" {
		where = append(where, "c.id in (select component_id from component_licenses where license = ?)")
		args = append(args, f.License)
	}
	if len(where) == 0 {
		return "", nil, ErrEmptyFilter
	}

	query := `select s.id, s.name, c.name, c.version, c.purl, c.type, c.licenses
from components c
join sboms s on s.id = c.sbom_id
where ` + strings.Join(where, " and ") + `
order by s.id, c.name, c.version`
	return query, args, nil
}
