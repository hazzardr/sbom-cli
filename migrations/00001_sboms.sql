-- +goose Up
-- Raw SBOM documents. data holds the full document as SQLite JSONB and is the
-- source of truth; the columns alongside it are extracted at ingest time.
create table sboms (
    id integer primary key,
    format text not null check (format in ('cyclonedx', 'spdx')),
    spec_version text not null,
    document_id text not null default '',
    name text not null default '',
    source text not null default '',
    -- SHA-256 of the canonical JSON content, for deduplication (store.contentDigest).
    sha256 text not null unique,
    ingested_at text not null default (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    data blob not null check (json_valid(data, 4))
);

-- SQLite has no GIN-style index into JSON arrays, so the queryable fields of
-- each component are normalized into indexed tables.
create table components (
    id integer primary key,
    sbom_id integer not null references sboms (id) on delete cascade,
    name text not null collate nocase,
    version text not null default '',
    purl text not null default '',
    type text not null default '',
    licenses text not null default ''
);

create index components_name_version on components (name, version);
create index components_version on components (version);
create index components_sbom_id on components (sbom_id);

-- One row per license ID found in a component's license expressions.
create table component_licenses (
    component_id integer not null references components (id) on delete cascade,
    license text not null collate nocase,
    primary key (component_id, license)
) without rowid;

create index component_licenses_license on component_licenses (license);

-- +goose Down
drop table component_licenses;
drop table components;
drop table sboms;
