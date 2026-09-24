<script lang="ts">
  import { setup, ApiError } from '../api/client'
  import { session } from '../lib/session.svelte'

  let serverName = $state('My Video Server')
  let username = $state('')
  let password = $state('')
  let confirmPassword = $state('')
  // Editable display names for the roots discovered on disk, seeded from
  // session.roots (GET /api/config already lists them, folder-name
  // defaults, even before setup completes).
  let rootNames = $state<Record<string, string>>(
    Object.fromEntries(session.roots.map((r) => [r.id, r.name])),
  )
  let submitting = $state(false)
  let error = $state('')

  async function onSubmit(e: SubmitEvent) {
    e.preventDefault()
    error = ''

    if (password !== confirmPassword) {
      error = 'Passwords do not match.'
      return
    }
    if (!serverName.trim() || !username.trim() || !password) {
      error = 'Server name, username, and password are all required.'
      return
    }

    submitting = true
    try {
      const roots = Object.fromEntries(
        Object.entries(rootNames)
          .filter(([, name]) => name.trim() !== '')
          .map(([id, name]) => [id, { displayName: name.trim() }]),
      )
      await setup({ serverName: serverName.trim(), username: username.trim(), password, roots })
      await session.refresh()
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not complete setup. Is the server reachable?'
    } finally {
      submitting = false
    }
  }
</script>

<div class="onboarding">
  <form onsubmit={onSubmit}>
    <h1>Set up your video server</h1>
    <p class="intro">This runs once. You'll use this username and password to log in from now on.</p>

    <label>
      Server name
      <input type="text" bind:value={serverName} placeholder="My Homelab" required />
    </label>

    <label>
      Username
      <input type="text" bind:value={username} autocomplete="username" required />
    </label>

    <label>
      Password
      <input type="password" bind:value={password} autocomplete="new-password" required />
    </label>

    <label>
      Confirm password
      <input type="password" bind:value={confirmPassword} autocomplete="new-password" required />
    </label>

    {#if session.roots.length > 0}
      <fieldset>
        <legend>Media libraries found</legend>
        <p class="hint">Give each one a friendly name, or leave it as-is.</p>
        {#each session.roots as root (root.id)}
          <label class="root-name">
            {root.id}
            <input type="text" bind:value={rootNames[root.id]} />
          </label>
        {/each}
      </fieldset>
    {:else}
      <p class="hint no-roots">
        No media folders found yet under the configured MEDIA_ROOT. That's fine — mount one and use
        "Rescan" from Settings once you're logged in.
      </p>
    {/if}

    {#if error}
      <p class="error">{error}</p>
    {/if}

    <button type="submit" disabled={submitting}>{submitting ? 'Setting up…' : 'Finish setup'}</button>
  </form>
</div>

<style>
  .onboarding {
    flex: 1;
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding: 48px 16px;
    overflow-y: auto;
  }

  form {
    width: 100%;
    max-width: 420px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  h1 {
    font-size: 1.3rem;
    margin: 0;
  }

  .intro {
    margin: 0 0 8px;
    color: var(--text-secondary);
    font-size: 0.9rem;
  }

  label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 0.85rem;
    color: var(--text-secondary);
  }

  input {
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
    font-size: 0.95rem;
  }

  fieldset {
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  legend {
    padding: 0 4px;
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--text);
  }

  .hint {
    margin: 0;
    font-size: 0.8rem;
    color: var(--text-tertiary);
  }

  .no-roots {
    padding: 10px;
    border-radius: 8px;
    background: var(--bg-elevated);
  }

  .root-name {
    flex-direction: row;
    align-items: center;
    gap: 8px;
    font-family: monospace;
  }

  .root-name input {
    flex: 1;
    font-family: inherit;
  }

  .error {
    margin: 0;
    color: var(--danger);
    font-size: 0.85rem;
  }

  button {
    margin-top: 8px;
    padding: 10px;
    border-radius: 8px;
    border: none;
    background: var(--accent);
    color: white;
    font-size: 0.95rem;
    font-weight: 600;
    cursor: pointer;
  }

  button:disabled {
    opacity: 0.6;
    cursor: default;
  }
</style>
