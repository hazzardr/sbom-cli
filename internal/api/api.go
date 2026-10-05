// Package api serves ingest and query over HTTP, mirroring the CLI forms:
//
//	POST /sboms                                     body: SBOM document
//	GET  /components?component=<name>[&version=<version>]
//	GET  /components?license=<license>
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/hazzardr/sbom-cli/internal/sbom"
	"github.com/hazzardr/sbom-cli/internal/store"
)

// MaxSBOMBytes caps the size of an ingested document.
const MaxSBOMBytes = 100 << 20

// New returns the API handler backed by s.
func New(s *store.Store) http.Handler {
	h := &handler{store: s}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /sboms", h.ingest)
	mux.HandleFunc("GET /components", h.query)
	return mux
}

type handler struct {
	store *store.Store
}

type ingestResponse struct {
	ID         int64  `json:"id"`
	Format     string `json:"format"`
	Components int    `json:"components"`
	Duplicate  bool   `json:"duplicate"`
}

// ingest stores the request body as an SBOM. It responds 201 for a new
// document and 200 when the same content was already stored. The optional
// source query parameter is recorded like the CLI's file path.
func (h *handler) ingest(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxSBOMBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "SBOM exceeds the size limit")
			return
		}
		writeError(w, http.StatusBadRequest, "read request body: "+err.Error())
		return
	}
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "api"
	}

	res, err := h.store.Ingest(r.Context(), source, raw)
	switch {
	case errors.Is(err, sbom.ErrMalformed):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, sbom.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	case err != nil:
		slog.ErrorContext(r.Context(), "ingest failed", "source", source, "error", err)
		writeError(w, http.StatusInternalServerError, "ingest failed")
		return
	}

	status := http.StatusCreated
	if res.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, ingestResponse{
		ID:         res.ID,
		Format:     string(res.Format),
		Components: res.Components,
		Duplicate:  res.Duplicate,
	})
}

func (h *handler) query(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.Filter{Component: q.Get("component"), Version: q.Get("version"), License: q.Get("license")}
	if err := validateQuery(f); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	matches, err := h.store.Search(r.Context(), f)
	if err != nil {
		slog.ErrorContext(r.Context(), "query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	if matches == nil {
		matches = []store.Match{}
	}
	writeJSON(w, http.StatusOK, matches)
}

// validateQuery enforces the same two forms as the CLI's query command.
func validateQuery(f store.Filter) error {
	switch {
	case f.Component != "" && f.License != "":
		return errors.New("use either component or license, not both")
	case f.Version != "" && f.Component == "":
		return errors.New("version requires component")
	case f.Component == "" && f.License == "":
		return errors.New("one of component or license is required")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
