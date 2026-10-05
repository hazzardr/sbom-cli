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

## Scaling to more formats / versions / BOM types (plan)

Every parser produces the format-neutral sbom.Document; store + queries only see that, so most additions are parser + fixtures.

New versions of an existing format
- additive versions (cyclonedx minors): add to cycloneDXSpecVersions + a real fixture + diff the schema for the fields we read
- decide policy: explicit allowlist (current, every version backed by a fixture) vs "any 1.x"
- cyclonedx 1.4/1.5 could reuse the current parser (same field shapes) if we need them
- breaking versions get their own parser: spdx 2.x is a different format (plain json, packages[], licenses on the package); github's dependency-graph export is spdx 2.3

Other formats / providers
- replace the hard-coded switch in sbom.Parse with a registry of {name, detect, parse}
- non-json serializations (cyclonedx xml/protobuf, spdx tag-value/yaml/rdf) clash with jsonb storage: convert to json at ingest (stored doc != original bytes) vs store raw bytes + media type. leaning convert
- revisit official libs (cyclonedx-go reads xml + json; check versions covered; check for a maintained spdx 3 go lib) once we read more than ~5 fields
- remote sources (s3/http, above) are separate from formats: resolve the argument to a reader, parsing unchanged

Other BOM types
- hbom / cbom / ml-bom / mbom: mostly components with a different `type`, already indexed; their extra details need new columns/tables to be queryable
- saasbom: services are a separate list => services table
- vex / vdr: vulnerabilities + links to components (incl. bom-link across documents) => vulnerabilities table
- dependency graph (all types) is dropped today => dependencies table for "contains X transitively"
- jsonb pays off here: a `reindex` command re-runs parsers over sboms.data to fill new tables, no re-ingest

First steps (small, format-agnostic)
1. drop the `format` check constraint (new migration 00002; sqlite can't alter a check, so it's a table rebuild that only gets more expensive with data)
2. parser registry
3. golden-file fixtures: testdata/<file> + expected Document as json, one table test, -update flag; adding a fixture needs no hand-written go
4. reindex + new tables only when a specific BOM type / query needs them

## Shared components / multi-tenant duplication

Decision (2026-10-05):
- consider: cross-tenant deduplication of whole documents (store each sbom document once, per-tenant ingestion records), designed before production
- don't worry about: intra-tenant duplication of components shared across a tenant's sboms; keep one component row per sbom. revisit only if the overlap query below shows high overlap on real data and index size matters

Concern: the same component is stored once per sbom it appears in; tenants ingesting the same sbom duplicate everything.

Measured (500k component occurrences, 200 sboms; sqlite; r = occurrences per distinct component). Index tables only; raw docs were a further ~153MB at this scale:

| r | current | full normalization (licenses on shared package) | hybrid (shared identity, licenses per occurrence) |
|---|---|---|---|
| 1 | 82MB | 102MB | 110MB |
| 2 | 81MB | 56MB | 77MB |
| 10 | 80MB | 19MB | 50MB |
| 50 | 79MB | 11MB | 44MB |

- the bigger duplication is raw documents (~64% of the db), which component dedup doesn't touch. at r=10, full normalization cuts the whole db ~28%, hybrid ~16%
- with no overlap (r=1) normalization is larger (extra join rows + indexes)
- "which sboms contain X" is the same speed either way (sub-ms; same rows touched). "distinct packages with license Y" gets faster (20ms -> ~0 at r=10)
- scale check: millions of occurrences = a few GB including docs; fine for one postgres (or sqlite). not a capacity problem at thousands of sboms

Why full normalization is harder than it looks (seen in our real fixtures)
- licenses are claims by the sbom author, not facts about the package: tomcat-catalina@9.0.14 is "Apache-2.0" in one fixture and 4 licenses in another; tools disagree. sharing licenses loses who-said-what => licenses stay per occurrence => only the hybrid's savings are realistic
- identity: purl is the only safe key and is often missing (3 of 5 fixtures have components without one). name+version is wrong: com.acme/tomcat-catalina@9.0.14 and org.apache.tomcat/tomcat-catalina@9.0.14 share name+version but are different packages (a fork)
- deletes need orphan cleanup (ref counting / sweeps)

Multi-tenancy is the real driver, and it's document-level, not component-level
- split "document" (content-addressed by the canonical sha256 we already compute; shared) from "ingestion" (tenant, source, name, ingested_at; per tenant). identical docs across tenants are stored + indexed once for free
- today dedupe is global: tenant B ingesting tenant A's doc would get "already ingested as SBOM N" — wrong (B gets no access) and an existence leak. responses must not reveal other tenants' documents
- every query must filter through tenant ownership
- `source` already belongs per ingestion: a duplicate ingest from a new path loses that path today
- regenerated sboms (new timestamp/serial) won't dedupe at document level; that's where component-level sharing would help

Recommendation
1. before production: design the document/ingestion (tenant) split; it changes dedupe semantics and every query
2. if raw doc storage hurts first: move docs to object storage keyed by sha256 (s3), keep the index in the db
3. if per-package enrichment is needed (vulns, license analysis): packages(purl unique) catalog + nullable package_id on component rows; no entity resolution needed for purl-less components
4. full component normalization only if measured overlap on real data is high and index size matters. measure with:
   select count(*) occurrences, count(distinct nullif(purl, '')) distinct_purls, sum(purl = '') no_purl from components;
