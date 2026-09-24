<script lang="ts">
  import Sidebar from './components/Sidebar.svelte'
  import TopBar from './components/TopBar.svelte'
  import StateMessage from './components/StateMessage.svelte'
  import { router, browsePath } from './lib/router.svelte'
  import { session } from './lib/session.svelte'
  import Browse from './routes/Browse.svelte'
  import Login from './routes/Login.svelte'
  import NotFound from './routes/NotFound.svelte'
  import Onboarding from './routes/Onboarding.svelte'
  import Recent from './routes/Recent.svelte'
  import Search from './routes/Search.svelte'
  import Settings from './routes/Settings.svelte'
  import Watch from './routes/Watch.svelte'

  $effect(() => {
    session.refresh()
  })

  // Once authenticated, "/" picks the first available root rather than
  // showing a blank page — but never silently for a deep link into a
  // specific root/folder, and never while there's nothing to redirect to
  // yet — a "no roots found yet" empty state matters more than a false
  // redirect.
  $effect(() => {
    if (router.route.name !== 'home' || !session.authenticated || session.roots.length === 0) return
    router.navigate(browsePath(session.roots[0].id, ''), { replace: true })
  })
</script>

{#if session.loading}
  <main class="centered"><StateMessage title="Loading…" /></main>
{:else if !session.configured}
  <Onboarding />
{:else if !session.authenticated}
  <Login />
{:else}
  <div class="shell">
    <Sidebar />

    <div class="content-column">
      <TopBar />

      <main>
        {#if router.route.name === 'browse'}
          <Browse
            root={router.route.root}
            path={router.route.path}
            view={router.route.view}
            sort={router.route.sort}
            flatten={router.route.flatten}
            tagIds={router.route.tagIds}
          />
        {:else if router.route.name === 'watch'}
          <Watch id={router.route.id} />
        {:else if router.route.name === 'search'}
          <Search query={router.route.query} />
        {:else if router.route.name === 'settings'}
          <Settings />
        {:else if router.route.name === 'recent'}
          <Recent view={router.route.view} tagIds={router.route.tagIds} />
        {:else if router.route.name === 'home'}
          {#if session.roots.length === 0}
            <StateMessage
              title="No media libraries found yet"
              detail="Mount a folder under MEDIA_ROOT, then use Settings → Rescan."
            />
          {:else}
            <StateMessage title="Loading…" />
          {/if}
        {:else}
          <NotFound />
        {/if}
      </main>
    </div>
  </div>
{/if}

<style>
  .shell {
    flex: 1;
    min-height: 0;
    display: flex;
  }

  .content-column {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  main {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    /* The scroll container: a long folder listing (Browse) scrolls here
       rather than growing #app past the viewport. The player route
       (Watch) is sized to fit exactly within this instead — see its own
       styles — so this never actually needs to scroll on that route. */
    overflow-y: auto;
  }

  .centered {
    align-items: center;
    justify-content: center;
  }
</style>
