package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hazzardr/sbom-cli/internal/store"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return New(s)
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "sbom", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func do(t *testing.T, h http.Handler, method, target string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, target, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestIngest(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	raw := fixture(t, "cyclonedx-1.6-spec-valid-bom.json")

	rec := do(t, h, http.MethodPost, "/sboms?source=valid-bom.json", raw)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first ingest: status %d, body %s", rec.Code, rec.Body)
	}
	var got ingestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := ingestResponse{ID: 1, Format: "cyclonedx", Components: 4}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}

	rec = do(t, h, http.MethodPost, "/sboms", raw)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"duplicate":true`) {
		t.Errorf("re-ingest: status %d, body %s", rec.Code, rec.Body)
	}
}

func TestIngestErrors(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	tests := []struct {
		name string
		body []byte
		want int
	}{
		{"unsupported format", fixture(t, "unsupported/cyclonedx-1.4-examples-laravel.json"), http.StatusUnprocessableEntity},
		{"malformed json", []byte(`{"bomFormat": "CycloneDX",`), http.StatusBadRequest},
	}
	for _, tt := range tests {
		if rec := do(t, h, http.MethodPost, "/sboms", tt.body); rec.Code != tt.want {
			t.Errorf("%s: status %d, want %d (body %s)", tt.name, rec.Code, tt.want, rec.Body)
		}
	}
}

func TestQuery(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	for _, name := range []string{"cyclonedx-1.6-spec-valid-bom.json", "spdx-3.0.1-examples-example11.json"} {
		if rec := do(t, h, http.MethodPost, "/sboms", fixture(t, name)); rec.Code != http.StatusCreated {
			t.Fatalf("ingest %s: status %d", name, rec.Code)
		}
	}

	tests := []struct {
		target string
		want   []string
	}{
		{"/components?component=tomcat-catalina", []string{"tomcat-catalina@9.0.14"}},
		{"/components?component=Acme+Application&version=9.1.1", []string{"Acme Application@9.1.1"}},
		{"/components?license=MIT", []string{"hyper@0.14", "pretty_env_logger@0.4.0", "tokio@1.19.2"}},
		{"/components?component=does-not-exist", []string{}},
	}
	for _, tt := range tests {
		rec := do(t, h, http.MethodGet, tt.target, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d, body %s", tt.target, rec.Code, rec.Body)
			continue
		}
		var matches []store.Match
		if err := json.Unmarshal(rec.Body.Bytes(), &matches); err != nil {
			t.Fatalf("%s: %v", tt.target, err)
		}
		got := []string{}
		for _, m := range matches {
			got = append(got, m.Component+"@"+m.Version)
		}
		if strings.Join(got, ",") != strings.Join(tt.want, ",") {
			t.Errorf("%s: got %v, want %v", tt.target, got, tt.want)
		}
	}
}

func TestQueryValidation(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	tests := map[string]string{
		"/components":                         "one of component or license is required",
		"/components?version=1.0":             "version requires component",
		"/components?license=MIT&version=1.0": "version requires component",
		"/components?component=x&license=MIT": "use either component or license",
		"/components?component=&license=":     "one of component or license is required",
	}
	for target, want := range tests {
		rec := do(t, h, http.MethodGet, target, nil)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: status %d, body %s; want 400 containing %q", target, rec.Code, rec.Body, want)
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	if rec := do(t, h, http.MethodGet, "/sboms", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /sboms: status %d, want 405", rec.Code)
	}
}
