// Package migrations embeds the goose SQL migrations so the binary can
// migrate its database without the source tree.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
