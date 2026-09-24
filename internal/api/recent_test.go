package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type recentResponse struct {
	Videos []struct {
		ID   string `json:"id"`
		Root string `json:"root"`
		Name string `json:"name"`
	} `json:"videos"`
}

func TestRecentEndpointListsIndexedVideos(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, idx := newTestServerWithIndexer(t, root)
	idx.Scan(t.Context())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/recent", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp recentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(resp.Videos) != 1 || resp.Videos[0].Name != "a.mp4" {
		t.Fatalf("expected a.mp4 in recently-added, got %+v", resp.Videos)
	}
	if resp.Videos[0].Root != "main" {
		t.Fatalf("expected root=main on the recent entry, got %q", resp.Videos[0].Root)
	}
}

func TestRecentEndpointEmptyWithoutIndex(t *testing.T) {
	root := t.TempDir()
	handler, _ := newTestServerWithIndexer(t, root) // no Scan() run
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/recent", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp recentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Videos) != 0 {
		t.Fatalf("expected no videos before any scan, got %+v", resp.Videos)
	}
}

func TestRecentEndpointRejectsUnknownRoot(t *testing.T) {
	root := t.TempDir()
	handler, idx := newTestServerWithIndexer(t, root)
	idx.Scan(t.Context())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/recent?root=bogus", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestRecentEndpointRespectsLimit(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.mp4", "b.mp4", "c.mp4"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	handler, idx := newTestServerWithIndexer(t, root)
	idx.Scan(t.Context())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/recent?limit=2", nil))
	var resp recentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Videos) != 2 {
		t.Fatalf("expected 2 videos (limit), got %d", len(resp.Videos))
	}
}

func TestRecentEndpointFilterByTag(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, idx := newTestServerWithIndexer(t, root)
	idx.Scan(t.Context())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=", nil))
	var browse struct {
		Videos []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &browse); err != nil || len(browse.Videos) != 2 {
		t.Fatalf("unexpected browse response: %s (err %v)", rec.Body.String(), err)
	}
	var aID string
	for _, v := range browse.Videos {
		if v.Name == "a.mp4" {
			aID = v.ID
		}
	}

	body, _ := json.Marshal(map[string]string{"name": "Personal"})
	tagRec := httptest.NewRecorder()
	handler.ServeHTTP(tagRec, httptest.NewRequest(http.MethodPost, "/api/videos/"+aID+"/tags", bytes.NewReader(body)))
	var tag struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(tagRec.Body.Bytes(), &tag); err != nil {
		t.Fatal(err)
	}

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/recent?tags="+itoa(tag.ID), nil))
	var resp recentResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Videos) != 1 || resp.Videos[0].Name != "a.mp4" {
		t.Fatalf("expected only a.mp4 when filtering recent by its tag, got %+v", resp.Videos)
	}
}
