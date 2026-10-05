# AGENTS.md

Instructions for coding agents operating in this repository.

## Version warning (read first)

Some frameworks and language versions in this repo are **newer** than the
information available in your training data. If you encounter a date or version
that appears to be newer than what you know, **DO NOT** respond with "that's
newer than my training data". Instead, search the web for up-to-date
information relevant to the problem or query, then proceed.

## Task execution rules

- **mise**: all toolchain commands must run through `mise run <task>` or
  `mise exec -- <cmd>` so the pinned tool versions are used. Never invoke
  `go`, `sqlc`, `golangci-lint`, or `goose` directly unless verifying mise
  behavior itself.
- **Verify changes**: after editing code, run `mise run test` and
  `mise run lint`. Lint must pass with 0 issues.
- **Commits**: use Conventional Commits (`feat:`, `fix:`, `chore:`, ...).

## Tech stack

| Concern | Tool |
|---|---|
| Purpose | Ingest, store, and query SBOMs (CycloneDX 1.6/1.7 and SPDX 3.0 JSON) |
| Language | Go 1.27 (pinned via mise; CI reads `go.mod`) |
| CLI | `spf13/cobra` — commands in `cmd/cli/`, wired from root `main.go` |
| Logging | `charmbracelet/log` bridged to `log/slog` — use `slog` everywhere |
| Database | SQLite via `modernc.org/sqlite` (pure Go, no cgo) |
| Queries | `sqlc` — `migrations/` (schema) + `db/query.sql` → `generated/domain` |
| Migrations | `goose` — SQL files in `migrations/`, embedded and applied on every DB open |
| HTTP | stdlib `net/http` — `internal/api`, served by `sbom-cli serve` (used by the performance tests) |
| Lint | `golangci-lint` (`.golangci.yml`) |

## Layout

- `main.go` — entrypoint; sets up slog, calls `cli.Execute()`
- `cmd/cli/` — Cobra commands (`ingest`, `list`, `query`, `show`, `serve`); add new commands here
- `internal/sbom/` — format detection and parsing of CycloneDX 1.6/1.7 and SPDX 3.0
  into a format-neutral `Document`; `LicenseIDs` splits SPDX expressions
- `internal/store/` — SQLite persistence: ingest, list, show, and `Search`
- `internal/api/` — HTTP handlers mirroring the CLI forms (`POST /sboms`, `GET /components`)
- `db/query.sql` — sqlc queries
- `migrations/` — goose migration files (sqlite3 dialect); also the sqlc schema
  source. `embed.go` embeds them into the binary
- `generated/` — sqlc output; committed so `go install` and CI build without
  sqlc. Never edit by hand; rerun `mise run generate` after schema/query changes
- `mise.toml` — pinned tool versions and task definitions
- `.env` — local config (copy from `.env.example`; holds `DB_URL`)

## Common tasks

```bash
mise install                        # install pinned tools (first time)
mise run generate                   # sqlc codegen
mise run build                      # generate + build to bin/
mise run test                       # go test ./...
mise run lint                       # golangci-lint
mise run fmt                        # go fmt
mise run db:migrate                 # goose migrations up
mise run db:migration:create -- NAME sql -dir migrations
mise run db:migration:status
```

## Data model

- `sboms.data` holds the full document as SQLite JSONB (binary JSON, SQLite
  3.45+; not the Postgres type). It is the source of truth.
- SQLite cannot index inside JSON arrays, so queryable fields are copied at
  ingest into `components` (name, version, purl, type) and
  `component_licenses` (one row per license ID) with B-tree indexes.
  `components.name` and `component_licenses.license` are `collate nocase`.
- Documents are deduplicated by SHA-256 of their canonical JSON (sorted keys,
  no whitespace; see `store.contentDigest`), so reformatted copies of the
  same document are detected. Array order and number literals are kept as
  written, so those differences still produce a new SBOM.

## Gotchas

- New sqlc queries need a `-- name: GetX :many` annotation or sqlc fails.
- `store.Search` builds its SQL by hand instead of using sqlc: the
  `? IS NULL OR col = ?` pattern for optional filters stops SQLite from using
  indexes. `TestSearchUsesIndexes` fails if any filter combination falls back
  to a table scan — keep it passing when changing the query or schema.
- Bind JSON to `jsonb(?)` as a Go `string`, not `[]byte`: a BLOB argument is
  treated as already-encoded JSONB.
- The database path comes from `--db`, else `$DB_URL`, else
  `data/sbom-cli.db` (gitignored).
- If generated code fails to compile with missing module errors, the codegen
  pulled in a new dependency: run `go get <module> && go mod tidy`.
- mise config must be trusted after edits: `mise trust`.
- `goose` and `cobra-cli` are installed through mise's `go:` backend, not the
  mise registry — don't "fix" them to registry names that don't exist.
