package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestHandleSearch(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "Shows"))
	mustWriteFile(t, filepath.Join(root, "Shows", "The Great Escape.mp4"))
	mustWriteFile(t, filepath.Join(root, "vacation clip.mp4"))
	mustWriteFile(t, filepath.Join(root, "unrelated.mp4"))

	handler, idx := newTestServerWithIndexer(t, root)
	idx.Scan(t.Context())

	var results struct {
		Query   string `json:"query"`
		Results []struct {
			Name       string `json:"name"`
			Root       string `json:"root"`
			FolderPath string `json:"folderPath"`
		} `json:"results"`
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=escape", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decoding search response: %v", err)
	}
	if len(results.Results) != 1 || results.Results[0].Name != "The Great Escape.mp4" {
		t.Fatalf("unexpected results for %q: %+v", "escape", results.Results)
	}
	if results.Results[0].Root != "main" || results.Results[0].FolderPath != "Shows" {
		t.Fatalf("unexpected root/folderPath: %+v", results.Results[0])
	}

	// Case-insensitive.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=VACATION", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decoding search response: %v", err)
	}
	if len(results.Results) != 1 || results.Results[0].Name != "vacation clip.mp4" {
		t.Fatalf("unexpected case-insensitive results: %+v", results.Results)
	}

	// No match.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=doesnotexist", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decoding search response: %v", err)
	}
	if len(results.Results) != 0 {
		t.Fatalf("expected no results, got %+v", results.Results)
	}

	// Blank query returns no results, not an error.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("blank query status = %d", rec.Code)
	}
}

func TestHandleSearchUnindexedFileAbsent(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "not yet indexed.mp4"))

	// No scan run — nothing in the index yet.
	handler := newTestServer(t, root)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=indexed", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var results struct {
		Results []any `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(results.Results) != 0 {
		t.Fatalf("expected no crash and no results for an unindexed file, got %+v", results.Results)
	}
}
