# sbom-cli

Ingest, store, and query software bills of materials. Supports CycloneDX 1.6/1.7
and SPDX 3.0 JSON. See [AGENTS.md](AGENTS.md) for the tech stack, data model,
and agent operating instructions.

```bash
mise install       # install pinned tools (go, sqlc, goose, golangci-lint, cobra-cli, lefthook)
mise exec -- lefthook install  # install git hooks
cp .env.example .env
mise run build
./bin/sbom-cli --help
```

## Usage

```bash
sbom-cli ingest <sbom-file>                                # - reads stdin
sbom-cli query --component <name> [--version <version>]
sbom-cli query --license <license>                         # MIT matches "MIT OR Apache-2.0"
```

`query` accepts `--json`. `sbom-cli list` and `sbom-cli show <id>` list
stored SBOMs and print one back as JSON.

The database is created and migrated automatically at `--db`, `$DB_URL`, or
`data/sbom-cli.db`.
