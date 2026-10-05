# sbom-cli

Ingest, store, and query software bills of materials. Supports CycloneDX 1.6
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
sbom-cli ingest app.cdx.json service.spdx3.json   # or - for stdin
sbom-cli list
sbom-cli query --component log4j-core
sbom-cli query --component log4j-core --version 2.14.1
sbom-cli query --license MIT --json               # matches "MIT OR Apache-2.0" too
sbom-cli show 1                                   # print a stored SBOM
```

The database is created and migrated automatically at `--db`, `$DB_URL`, or
`data/sbom-cli.db`.
