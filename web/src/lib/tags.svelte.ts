// A small reactive registry of every tag in the library (name, color,
// video count), shared across the filter-by-tag row and every tag editor
// on the page — same hand-written-class pattern as router.svelte.ts and
// session.svelte.ts. Kept in one place so tagging a video from one card
// immediately updates the filter row's counts everywhere else, without a
// full page reload.
import { addTagToVideo, bulkTagVideos, deleteTag, getTags, removeTagFromVideo } from '../api/client'
import type { TagWithCount } from '../api/types'

class TagRegistry {
  tags = $state<TagWithCount[]>([])
  loaded = $state(false)

  async refresh(): Promise<void> {
    try {
      const res = await getTags()
      this.tags = res.tags
    } finally {
      this.loaded = true
    }
  }

  async addToVideo(videoId: string, name: string) {
    const tag = await addTagToVideo(videoId, name)
    await this.refresh()
    return tag
  }

  async removeFromVideo(videoId: string, tagId: number) {
    await removeTagFromVideo(videoId, tagId)
    await this.refresh()
  }

  async bulkAdd(videoIds: string[], name: string) {
    const tag = await bulkTagVideos(videoIds, name)
    await this.refresh()
    return tag
  }

  async remove(tagId: number) {
    await deleteTag(tagId)
    await this.refresh()
  }
}

export const tagRegistry = new TagRegistry()
