- cyclonedx "second edition" pdf actually links to third edition (can't find source for 1.6, third edition is 1.7): https://cyclonedx.org/guides/
- add pagination
- add remote sbom source (s3/http/etc)
- scaling to multiple document types (already have 2)

## Concerns / future work

Concurrency (accepted PoC tradeoff)
- sqlite is single-writer: concurrent ingests serialize on the write lock; busy_timeout is 5s, so a long ingest can make others fail with SQLITE_BUSY
- multi-writer service => move to postgres (sqlc supports it; schema ports cleanly)
- migrations run on every open; two processes opening a fresh db at once could race on goose's version table (not verified)
- write txs are deferred; fine while the first statement is a write, use _txlock=immediate if a read is ever added first

Dedup
- revisions / regenerated sboms of the same software are stored as new sboms; needs a policy (replace? keep both? latest-only in queries?) and storing cyclonedx `version`
- don't dedupe on serialNumber / spdx document id alone: real examples reuse ids (cyclonedx spec test bom + guide example share one serialNumber)
- canonical hash keeps array order and number literals as written (1 vs 1.0 => different sbom)
- dbs created before canonical hashing hold raw-byte hashes; no backfill

Performance (measured: 200 sboms x 2500 components, ~500k component rows)
- ingest ~110-140ms per 2500-component doc, dominated by row inserts => prepared statements / multi-row inserts
- component + license query: 23ms with no matches (sqlite gathers all ids for the license first) => rewrite as join + ANALYZE after ingest
- pagination (above): --license MIT returned 100k rows in ~170ms; also a count-only mode
- json is decoded 3x per ingest (format probe, parse, canonical digest; digest ~6.5ms per 0.77MB) => merge
- full docs are ~64% of db size

Parsing / query gaps
- 3 of 5 real fixtures have no document name => blank names in list/query; fall back to root component or file name
- cyclonedx 1.7 `versionRange` components get an empty version
- version matching is exact only; ranges (e.g. log4j < 2.17) need per-ecosystem version comparison
- no partial name match; purl stored but not indexed or queryable
- not indexed: spdx files/snippets, cyclonedx services, pedigree ancestors (intended)
- rejected: cyclonedx xml, cyclonedx < 1.6, spdx 2.x
- no delete command
- ingest takes one file (acceptance criteria) => bulk loads re-open the db and re-check migrations per file

Tooling / tests
- ci workflow changes (mise-action, generated-code check, go-version-file, pinned golangci-lint) haven't run on github yet: no remote; validated locally with actionlint + the same shell steps
- golangci-lint version is pinned twice (mise.toml + ci.yml lint job); keep in sync
- cyclonedx guide fixture was transcribed from pdf text; no downloadable json source exists
