package database_test

import (
	"context"
	"testing"
)

func TestCreateOrGetTagIsIdempotentAndCaseInsensitive(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	a, err := store.CreateOrGetTag(ctx, "Personal")
	if err != nil {
		t.Fatalf("CreateOrGetTag error: %v", err)
	}
	if a.ID == 0 || a.Name != "Personal" || a.Color == "" {
		t.Fatalf("unexpected tag: %+v", a)
	}

	// Same name, different case: must return the same tag, not create a
	// second one (freeform tagging must not let "Personal" and "personal"
	// silently diverge into two tags).
	b, err := store.CreateOrGetTag(ctx, "personal")
	if err != nil {
		t.Fatalf("CreateOrGetTag (repeat) error: %v", err)
	}
	if b.ID != a.ID {
		t.Fatalf("expected the same tag ID for a case-different repeat, got %d and %d", a.ID, b.ID)
	}

	tags, err := store.ListTagsWithCounts(ctx)
	if err != nil {
		t.Fatalf("ListTagsWithCounts error: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected exactly 1 tag after a case-different repeat, got %+v", tags)
	}
}

func TestCreateOrGetTagRejectsEmptyName(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.CreateOrGetTag(context.Background(), "   "); err == nil {
		t.Fatal("expected an error for a blank tag name")
	}
}

func TestCreateOrGetTagColorIsDeterministic(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tag, err := store.CreateOrGetTag(ctx, "Archive")
	if err != nil {
		t.Fatalf("CreateOrGetTag error: %v", err)
	}
	if err := store.DeleteTag(ctx, tag.ID); err != nil {
		t.Fatalf("DeleteTag error: %v", err)
	}
	recreated, err := store.CreateOrGetTag(ctx, "Archive")
	if err != nil {
		t.Fatalf("CreateOrGetTag (recreate) error: %v", err)
	}
	if recreated.Color != tag.Color {
		t.Fatalf("expected the same deterministic color after delete+recreate, got %q then %q", tag.Color, recreated.Color)
	}
}

func TestTagVideoLifecycle(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	personal, err := store.CreateOrGetTag(ctx, "Personal")
	if err != nil {
		t.Fatalf("CreateOrGetTag error: %v", err)
	}
	work, err := store.CreateOrGetTag(ctx, "Work")
	if err != nil {
		t.Fatalf("CreateOrGetTag error: %v", err)
	}

	if err := store.TagVideo(ctx, "video-1", personal.ID); err != nil {
		t.Fatalf("TagVideo error: %v", err)
	}
	if err := store.TagVideo(ctx, "video-1", personal.ID); err != nil { // idempotent
		t.Fatalf("TagVideo (repeat) error: %v", err)
	}
	if err := store.TagVideo(ctx, "video-1", work.ID); err != nil {
		t.Fatalf("TagVideo error: %v", err)
	}

	tags, err := store.GetVideoTags(ctx, "video-1")
	if err != nil {
		t.Fatalf("GetVideoTags error: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %+v", tags)
	}

	if err := store.UntagVideo(ctx, "video-1", personal.ID); err != nil {
		t.Fatalf("UntagVideo error: %v", err)
	}
	tags, err = store.GetVideoTags(ctx, "video-1")
	if err != nil {
		t.Fatalf("GetVideoTags error: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "Work" {
		t.Fatalf("expected only Work left, got %+v", tags)
	}
}

func TestGetTagsForVideosBatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	personal, err := store.CreateOrGetTag(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	work, err := store.CreateOrGetTag(ctx, "Work")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.TagVideo(ctx, "video-1", personal.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.TagVideo(ctx, "video-2", work.ID); err != nil {
		t.Fatal(err)
	}
	// video-3 is deliberately left untagged.

	byID, err := store.GetTagsForVideos(ctx, []string{"video-1", "video-2", "video-3"})
	if err != nil {
		t.Fatalf("GetTagsForVideos error: %v", err)
	}
	if len(byID["video-1"]) != 1 || byID["video-1"][0].Name != "Personal" {
		t.Fatalf("video-1 tags = %+v", byID["video-1"])
	}
	if len(byID["video-2"]) != 1 || byID["video-2"][0].Name != "Work" {
		t.Fatalf("video-2 tags = %+v", byID["video-2"])
	}
	if _, present := byID["video-3"]; present {
		t.Fatalf("expected video-3 absent from the map (no tags), got %+v", byID["video-3"])
	}
}

func TestGetTagsForVideosEmptyInput(t *testing.T) {
	store := newTestStore(t)
	byID, err := store.GetTagsForVideos(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetTagsForVideos error: %v", err)
	}
	if len(byID) != 0 {
		t.Fatalf("expected an empty map, got %+v", byID)
	}
}

func TestTagVideosBulk(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tag, err := store.CreateOrGetTag(ctx, "Raw Footage")
	if err != nil {
		t.Fatal(err)
	}

	ids := []string{"video-1", "video-2", "video-3"}
	if err := store.TagVideos(ctx, ids, tag.ID); err != nil {
		t.Fatalf("TagVideos error: %v", err)
	}

	byID, err := store.GetTagsForVideos(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if len(byID[id]) != 1 || byID[id][0].ID != tag.ID {
			t.Fatalf("expected %s tagged with %q, got %+v", id, tag.Name, byID[id])
		}
	}
}

func TestListTagsWithCounts(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	personal, err := store.CreateOrGetTag(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOrGetTag(ctx, "Archive"); err != nil { // 0 videos
		t.Fatal(err)
	}
	if err := store.TagVideo(ctx, "video-1", personal.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.TagVideo(ctx, "video-2", personal.ID); err != nil {
		t.Fatal(err)
	}

	tags, err := store.ListTagsWithCounts(ctx)
	if err != nil {
		t.Fatalf("ListTagsWithCounts error: %v", err)
	}
	byName := map[string]int{}
	for _, tg := range tags {
		byName[tg.Name] = tg.Count
	}
	if byName["Personal"] != 2 {
		t.Fatalf("Personal count = %d, want 2", byName["Personal"])
	}
	if byName["Archive"] != 0 {
		t.Fatalf("Archive count = %d, want 0 (tag with no videos must still be listed)", byName["Archive"])
	}
}

func TestDeleteTagCascadesToVideoTags(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tag, err := store.CreateOrGetTag(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.TagVideo(ctx, "video-1", tag.ID); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteTag(ctx, tag.ID); err != nil {
		t.Fatalf("DeleteTag error: %v", err)
	}

	tags, err := store.GetVideoTags(ctx, "video-1")
	if err != nil {
		t.Fatalf("GetVideoTags error: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected video-1's tags gone after DeleteTag (cascade), got %+v", tags)
	}
	all, err := store.ListTagsWithCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("expected no tags left, got %+v", all)
	}
}
