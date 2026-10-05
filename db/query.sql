-- name: InsertSBOM :one
insert into sboms (format, spec_version, document_id, name, source, sha256, data)
values (?, ?, ?, ?, ?, ?, jsonb(sqlc.arg(data)))
returning id;

-- name: InsertComponent :one
insert into components (sbom_id, name, version, purl, type, licenses)
values (?, ?, ?, ?, ?, ?)
returning id;

-- name: InsertComponentLicense :exec
insert or ignore into component_licenses (component_id, license)
values (?, ?);

-- name: GetSBOMIDBySHA256 :one
select id from sboms where sha256 = ?;

-- name: GetSBOMJSON :one
select cast(json(data) as text) as document from sboms where id = ?;

-- name: ListSBOMs :many
select
    s.id,
    s.format,
    s.spec_version,
    s.name,
    s.source,
    s.ingested_at,
    (select count(*) from components c where c.sbom_id = s.id) as component_count
from sboms s
order by s.id;
