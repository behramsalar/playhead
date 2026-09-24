package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func browseFirstVideoID(t *testing.T, handler http.Handler, query string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?"+query, nil))
	var browse struct {
		Videos []struct {
			ID string `json:"id"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &browse); err != nil || len(browse.Videos) == 0 {
		t.Fatalf("unexpected browse response: %s (err %v)", rec.Body.String(), err)
	}
	return browse.Videos[0].ID
}

type videoTagsResponse struct {
	Tags []struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	} `json:"tags"`
}

func TestTagVideoLifecycleThroughAPI(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := newTestServer(t, root)
	id := browseFirstVideoID(t, handler, "root=main&path=")

	// Tag it.
	body, _ := json.Marshal(map[string]string{"name": "Personal"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/videos/"+id+"/tags", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST tag status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var tag struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tag); err != nil {
		t.Fatalf("decoding tag: %v", err)
	}
	if tag.Name != "Personal" || tag.Color == "" {
		t.Fatalf("unexpected tag: %+v", tag)
	}

	// It shows up on the video info response.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videos/"+id, nil))
	var info videoTagsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decoding video info: %v", err)
	}
	if len(info.Tags) != 1 || info.Tags[0].Name != "Personal" {
		t.Fatalf("expected Personal tag on video info, got %+v", info.Tags)
	}

	// And on the browse listing.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=", nil))
	var browse struct {
		Videos []struct {
			Tags []struct {
				Name string `json:"name"`
			} `json:"tags"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &browse); err != nil {
		t.Fatalf("decoding browse: %v", err)
	}
	if len(browse.Videos) != 1 || len(browse.Videos[0].Tags) != 1 || browse.Videos[0].Tags[0].Name != "Personal" {
		t.Fatalf("expected Personal tag on browse listing, got %+v", browse.Videos)
	}

	// Remove it.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/videos/"+id+"/tags/"+itoa(tag.ID), nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE tag status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videos/"+id, nil))
	var infoAfter videoTagsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &infoAfter); err != nil {
		t.Fatalf("decoding video info: %v", err)
	}
	if len(infoAfter.Tags) != 0 {
		t.Fatalf("expected no tags after removal, got %+v", infoAfter.Tags)
	}
}

func TestTagNameIsCaseInsensitiveAcrossVideos(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.mp4"), []byte("data"), 0o644); err != nil {
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
	if err := json.Unmarshal(rec.Body.Bytes(), &browse); err != nil || len(browse.Videos) != 2 {
		t.Fatalf("unexpected browse response: %s (err %v)", rec.Body.String(), err)
	}

	body1, _ := json.Marshal(map[string]string{"name": "Work"})
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, httptest.NewRequest(http.MethodPost, "/api/videos/"+browse.Videos[0].ID+"/tags", bytes.NewReader(body1)))

	body2, _ := json.Marshal(map[string]string{"name": "work"})
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/api/videos/"+browse.Videos[1].ID+"/tags", bytes.NewReader(body2)))

	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/api/tags", nil))
	var list struct {
		Tags []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		} `json:"tags"`
	}
	if err := json.Unmarshal(rec3.Body.Bytes(), &list); err != nil {
		t.Fatalf("decoding tags list: %v", err)
	}
	if len(list.Tags) != 1 || list.Tags[0].Count != 2 {
		t.Fatalf("expected 1 tag shared by both videos, got %+v", list.Tags)
	}
}

func TestBrowseFilterByTag(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := newTestServer(t, root)

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
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=&tags="+itoa(tag.ID), nil))
	var filtered struct {
		Videos []struct {
			Name string `json:"name"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("decoding filtered browse: %v", err)
	}
	if len(filtered.Videos) != 1 || filtered.Videos[0].Name != "a.mp4" {
		t.Fatalf("expected only a.mp4 when filtering by its tag, got %+v", filtered.Videos)
	}
}

func TestBulkTagVideos(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.mp4"), []byte("data"), 0o644); err != nil {
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
	if err := json.Unmarshal(rec.Body.Bytes(), &browse); err != nil || len(browse.Videos) != 2 {
		t.Fatalf("unexpected browse response: %s (err %v)", rec.Body.String(), err)
	}
	ids := []string{browse.Videos[0].ID, browse.Videos[1].ID}

	body, _ := json.Marshal(map[string]any{"videoIds": ids, "name": "Raw Footage"})
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/api/tags/bulk", bytes.NewReader(body)))
	if rec2.Code != http.StatusOK {
		t.Fatalf("bulk tag status = %d, body = %s", rec2.Code, rec2.Body.String())
	}

	for _, id := range ids {
		rec3 := httptest.NewRecorder()
		handler.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/api/videos/"+id, nil))
		var info videoTagsResponse
		if err := json.Unmarshal(rec3.Body.Bytes(), &info); err != nil {
			t.Fatal(err)
		}
		if len(info.Tags) != 1 || info.Tags[0].Name != "Raw Footage" {
			t.Fatalf("expected %s tagged with Raw Footage, got %+v", id, info.Tags)
		}
	}
}

func TestDeleteTagRemovesFromAllVideos(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := newTestServer(t, root)
	id := browseFirstVideoID(t, handler, "root=main&path=")

	body, _ := json.Marshal(map[string]string{"name": "Archive"})
	tagRec := httptest.NewRecorder()
	handler.ServeHTTP(tagRec, httptest.NewRequest(http.MethodPost, "/api/videos/"+id+"/tags", bytes.NewReader(body)))
	var tag struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(tagRec.Body.Bytes(), &tag); err != nil {
		t.Fatal(err)
	}

	delRec := httptest.NewRecorder()
	handler.ServeHTTP(delRec, httptest.NewRequest(http.MethodDelete, "/api/tags/"+itoa(tag.ID), nil))
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE tag status = %d, body = %s", delRec.Code, delRec.Body.String())
	}

	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/tags", nil))
	var list struct {
		Tags []any `json:"tags"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Tags) != 0 {
		t.Fatalf("expected no tags left after delete, got %+v", list.Tags)
	}
}

func TestAddTagRejectsEmptyName(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := newTestServer(t, root)
	id := browseFirstVideoID(t, handler, "root=main&path=")

	body, _ := json.Marshal(map[string]string{"name": "  "})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/videos/"+id+"/tags", bytes.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
