package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// contentDigest returns the hex SHA-256 of raw's canonical JSON form, so
// documents that differ only in whitespace, line endings, object key order,
// or string escaping get the same digest.
//
// Array order and number literals are kept as written: reordering an array
// can change its meaning, and normalizing numbers through float64 could make
// distinct documents collide. Missing a duplicate is safer than merging two
// different SBOMs.
func contentDigest(raw []byte) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", fmt.Errorf("canonicalize JSON: %w", err)
	}
	// json.Marshal sorts object keys and emits no insignificant whitespace.
	canonical, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("canonicalize JSON: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}
