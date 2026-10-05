// Package store persists SBOMs in SQLite and answers component queries.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pressly/goose/v3"
	"modernc.org/sqlite" // also registers the "sqlite" database/sql driver
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/hazzardr/sbom-cli/generated/domain"
	"github.com/hazzardr/sbom-cli/internal/sbom"
	"github.com/hazzardr/sbom-cli/migrations"
)

// ErrNotFound is returned when a requested SBOM does not exist.
var ErrNotFound = errors.New("sbom not found")

// Store is a SQLite-backed SBOM repository.
type Store struct {
	db *sql.DB
	q  *domain.Queries
}

// Open opens (creating if needed) the database at path and applies any
// pending migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db, q: domain.New(db)}, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// IngestResult describes the outcome of ingesting one document.
type IngestResult struct {
	ID         int64
	Format     sbom.Format
	Components int
	// Duplicate is true when a document with the same content was already
	// stored (see contentDigest); ID then refers to the existing SBOM.
	Duplicate bool
}

// Ingest parses raw as an SBOM and stores it along with its component index.
// source records where the document came from, e.g. a file path.
func (s *Store) Ingest(ctx context.Context, source string, raw []byte) (IngestResult, error) {
	doc, err := sbom.Parse(raw)
	if err != nil {
		return IngestResult{}, err
	}
	digest, err := contentDigest(raw)
	if err != nil {
		return IngestResult{}, err
	}
	res := IngestResult{Format: doc.Format, Components: len(doc.Components)}

	// Check first so re-ingesting a known document skips the write path.
	id, found, err := s.findByDigest(ctx, digest)
	if err != nil {
		return IngestResult{}, err
	}
	if found {
		res.ID, res.Duplicate = id, true
		return res, nil
	}
	return s.insert(ctx, res, doc, source, digest, raw)
}

// insert writes a new document and its component index. If a concurrent
// ingest stored the same content after the duplicate check, the unique
// constraint on sboms.sha256 rejects this insert and the existing SBOM is
// reported as a duplicate instead of as an error.
func (s *Store) insert(ctx context.Context, res IngestResult, doc *sbom.Document,
	source, digest string, raw []byte,
) (IngestResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IngestResult{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res.ID, err = insertDocument(ctx, s.q.WithTx(tx), doc, source, digest, raw)
	if isUniqueViolation(err) {
		_ = tx.Rollback()
		id, found, lookupErr := s.findByDigest(ctx, digest)
		if lookupErr != nil {
			return IngestResult{}, lookupErr
		}
		if found {
			res.ID, res.Duplicate = id, true
			return res, nil
		}
	}
	if err != nil {
		return IngestResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return IngestResult{}, fmt.Errorf("commit: %w", err)
	}
	return res, nil
}

func (s *Store) findByDigest(ctx context.Context, digest string) (int64, bool, error) {
	id, err := s.q.GetSBOMIDBySHA256(ctx, digest)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("check for duplicate: %w", err)
	}
	return id, true, nil
}

func isUniqueViolation(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

func insertDocument(ctx context.Context, q *domain.Queries, doc *sbom.Document,
	source, digest string, raw []byte,
) (int64, error) {
	sbomID, err := q.InsertSBOM(ctx, domain.InsertSBOMParams{
		Format:      string(doc.Format),
		SpecVersion: doc.SpecVersion,
		DocumentID:  doc.DocumentID,
		Name:        doc.Name,
		Source:      source,
		Sha256:      digest,
		// Passed as text so jsonb() parses it; a []byte would be bound as a
		// BLOB and interpreted as already-encoded JSONB.
		Data: string(raw),
	})
	if err != nil {
		return 0, fmt.Errorf("insert sbom: %w", err)
	}

	for _, c := range doc.Components {
		componentID, err := q.InsertComponent(ctx, domain.InsertComponentParams{
			SbomID:   sbomID,
			Name:     c.Name,
			Version:  c.Version,
			Purl:     c.PURL,
			Type:     c.Type,
			Licenses: strings.Join(c.Licenses, ", "),
		})
		if err != nil {
			return 0, fmt.Errorf("insert component %q: %w", c.Name, err)
		}
		for _, license := range sbom.LicenseIDs(c.Licenses) {
			err := q.InsertComponentLicense(ctx, domain.InsertComponentLicenseParams{
				ComponentID: componentID,
				License:     license,
			})
			if err != nil {
				return 0, fmt.Errorf("insert license %q for %q: %w", license, c.Name, err)
			}
		}
	}
	return sbomID, nil
}

// SBOMSummary is a stored SBOM without its document body.
type SBOMSummary = domain.ListSBOMsRow

// List returns all stored SBOMs in ingestion order.
func (s *Store) List(ctx context.Context) ([]SBOMSummary, error) {
	rows, err := s.q.ListSBOMs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sboms: %w", err)
	}
	return rows, nil
}

// Document returns the stored SBOM as compact JSON.
func (s *Store) Document(ctx context.Context, id int64) (string, error) {
	doc, err := s.q.GetSBOMJSON(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: id %d", ErrNotFound, id)
	}
	if err != nil {
		return "", fmt.Errorf("get sbom %d: %w", id, err)
	}
	return doc, nil
}
