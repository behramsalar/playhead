<script lang="ts">
  // A lazily-expanding folder tree, rendered inside Sidebar.svelte's
  // "FOLDERS" section (which owns the surrounding chrome — width, mobile
  // drawer/backdrop, scrolling). Reuses the plain /api/browse endpoint for
  // each folder's children (ignoring its videos) rather than a dedicated
  // recursive endpoint, so it scales the same way the rest of the app
  // already does: only what's expanded gets fetched.
  import { browse, getRoots } from '../api/client'
  import { browsePath, type BrowseQuery } from '../lib/router.svelte'
  import { makeTreeNode, type TreeNode } from '../lib/tree'
  import FolderTreeNode from './FolderTreeNode.svelte'
  import Link from './Link.svelte'

  interface Props {
    root: string
    currentPath: string
    query: BrowseQuery
  }

  let { root, currentPath, query }: Props = $props()

  // Both effects below populate these properly (from the live `root`) as
  // soon as they run, which happens synchronously on mount — these are
  // just placeholders for the brief instant before that.
  let rootName = $state('')
  let rootNode = $state<TreeNode>(makeTreeNode('', ''))

  async function loadChildren(node: TreeNode) {
    if (node.children !== null || node.loading) return
    node.loading = true
    node.error = false
    try {
      const result = await browse(root, node.path)
      node.children = result.folders.map((f) => makeTreeNode(f.path, f.name))
    } catch {
      node.error = true
    } finally {
      node.loading = false
    }
  }

  async function toggle(node: TreeNode) {
    node.expanded = !node.expanded
    if (node.expanded) await loadChildren(node)
  }

  $effect(() => {
    const currentRoot = root
    let cancelled = false

    getRoots()
      .then(({ roots }) => {
        if (cancelled) return
        rootName = roots.find((r) => r.id === currentRoot)?.name ?? currentRoot
      })
      .catch(() => {
        // Non-critical: the tree still works with the raw root id as its label.
      })

    return () => {
      cancelled = true
    }
  })

  $effect(() => {
    // Rebuild from scratch on root change; re-walk down to the current
    // path (expanding+fetching each ancestor) whenever it changes too, so
    // navigating via breadcrumbs/cards/back-forward keeps the tree in sync
    // and reveals your location even the first time the sidebar opens.
    const currentRoot = root
    const targetPath = currentPath
    let cancelled = false

    // The node's `name` is never rendered (the template shows `rootName`
    // directly), so this doesn't need to depend on — or wait for — that
    // separate, best-effort fetch.
    //
    // Build the tree in `node`, a plain local object, and only ever WRITE
    // to `rootNode` (fresh shallow copies, below) — never read it back
    // inside this effect. Reading a $state variable that the same effect
    // also writes makes it a dependency of itself: the effect's own write
    // then re-triggers the effect, which writes again, forever
    // ("effect_update_depth_exceeded"). Writing a plain object directly
    // also doesn't work: assigning it wraps it in a reactive proxy, and
    // that proxy — not the original object — is what the template reads,
    // so mutating the original object afterwards silently doesn't update
    // the UI. Snapshotting into a fresh object at each checkpoint avoids
    // both: every assignment is a pure write, and the template always
    // gets a proxy of the current data.
    const node = makeTreeNode('', currentRoot)
    node.expanded = true
    rootNode = { ...node }

    ;(async () => {
      await loadChildren(node)
      if (cancelled) return
      rootNode = { ...node }
      if (!targetPath) return

      let current = node
      let accPath = ''
      for (const segment of targetPath.split('/')) {
        accPath = accPath ? `${accPath}/${segment}` : segment
        if (cancelled || !current.children) return
        const child = current.children.find((c) => c.path === accPath)
        if (!child) return
        child.expanded = true
        await loadChildren(child)
        if (cancelled) return
        rootNode = { ...node }
        current = child
      }
    })()

    return () => {
      cancelled = true
    }
  })

</script>

<!-- Anchor clicks bubble up to Sidebar's own delegated handler, which
     closes the mobile drawer — see Sidebar.svelte's onSidebarClick. -->
<nav class="tree" aria-label="Folder tree">
  <Link href={browsePath(root, '', query)} class="root-name{currentPath === '' ? ' current' : ''}">
    {rootName}
  </Link>
  {#if rootNode.expanded && rootNode.children}
    {#each rootNode.children as child (child.path)}
      <FolderTreeNode {root} node={child} {currentPath} {query} depth={1} onToggle={toggle} />
    {/each}
  {/if}
</nav>

<style>
  .tree {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-height: 0;
  }

  :global(.root-name) {
    display: block;
    padding: 5px 8px;
    margin-bottom: 4px;
    border-radius: 4px;
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  :global(.root-name:hover) {
    background: var(--bg-hover);
  }

  :global(.root-name.current) {
    background: var(--bg-hover);
  }
</style>
