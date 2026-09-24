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

func TestResumePositionSaveGetClear(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := newTestServer(t, root)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=", nil))
	var browse struct {
		Videos []struct {
			ID string `json:"id"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &browse); err != nil || len(browse.Videos) != 1 {
		t.Fatalf("unexpected browse response: %s (err %v)", rec.Body.String(), err)
	}
	id := browse.Videos[0].ID

	// No saved position yet.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videos/"+id, nil))
	var info struct {
		ResumeSeconds *float64 `json:"resumeSeconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decoding video info: %v", err)
	}
	if info.ResumeSeconds != nil {
		t.Fatalf("expected no resume position yet, got %v", *info.ResumeSeconds)
	}

	// Save one.
	body, _ := json.Marshal(map[string]float64{"positionSeconds": 42.5})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/videos/"+id+"/resume", bytes.NewReader(body)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT resume status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videos/"+id, nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decoding video info: %v", err)
	}
	if info.ResumeSeconds == nil || *info.ResumeSeconds != 42.5 {
		t.Fatalf("expected resumeSeconds=42.5, got %v", info.ResumeSeconds)
	}

	// Reject a negative position.
	badBody, _ := json.Marshal(map[string]float64{"positionSeconds": -1})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/videos/"+id+"/resume", bytes.NewReader(badBody)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for negative position, got %d", rec.Code)
	}

	// Clear it.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/videos/"+id+"/resume", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE resume status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videos/"+id, nil))
	// Fresh struct: unmarshaling into the same `info` used above would
	// leave its ResumeSeconds field untouched by this decode (the field
	// is omitted from the JSON entirely, via omitempty, so json.Unmarshal
	// never sees it to clear it) — a stale-read bug in the test, not
	// something this asserts about the handler.
	var cleared struct {
		ResumeSeconds *float64 `json:"resumeSeconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cleared); err != nil {
		t.Fatalf("decoding video info: %v", err)
	}
	if cleared.ResumeSeconds != nil {
		t.Fatalf("expected resume position cleared, got %v", *cleared.ResumeSeconds)
	}
}

func TestResumePositionUnknownVideo(t *testing.T) {
	root := t.TempDir()
	handler := newTestServer(t, root)

	body, _ := json.Marshal(map[string]float64{"positionSeconds": 1})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/videos/bWFpbi9taXNzaW5nLm1wNA/resume", bytes.NewReader(body)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unresolvable video id, got %d: %s", rec.Code, rec.Body.String())
	}
}
