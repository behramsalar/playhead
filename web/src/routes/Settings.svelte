<script lang="ts">
  import { changePassword, getSettings, logout, rescanRoots, updateSettings, ApiError } from '../api/client'
  import type { RootSetting } from '../api/types'
  import { router } from '../lib/router.svelte'
  import { session } from '../lib/session.svelte'

  let loading = $state(true)
  let loadError = $state('')
  let serverName = $state('')
  let username = $state('')
  let roots = $state<RootSetting[]>([])

  let savingGeneral = $state(false)
  let generalMessage = $state('')
  let generalError = $state('')

  let rescanning = $state(false)
  let rescanMessage = $state('')

  let currentPassword = $state('')
  let newUsername = $state('')
  let newPassword = $state('')
  let confirmNewPassword = $state('')
  let changingPassword = $state(false)
  let passwordError = $state('')

  async function load() {
    loading = true
    loadError = ''
    try {
      const s = await getSettings()
      serverName = s.serverName
      username = s.username
      roots = s.roots
    } catch (e) {
      loadError = e instanceof ApiError ? e.message : 'Could not load settings.'
    } finally {
      loading = false
    }
  }
  load()

  async function saveGeneral(e: SubmitEvent) {
    e.preventDefault()
    generalMessage = ''
    generalError = ''
    savingGeneral = true
    try {
      const overrides = Object.fromEntries(
        roots.map((r) => [r.id, { displayName: r.displayName.trim() || r.folderName, hidden: r.hidden }]),
      )
      const s = await updateSettings({ serverName: serverName.trim(), roots: overrides })
      serverName = s.serverName
      roots = s.roots
      generalMessage = 'Saved.'
      await session.refresh()
    } catch (e) {
      generalError = e instanceof ApiError ? e.message : 'Could not save settings.'
    } finally {
      savingGeneral = false
    }
  }

  async function onRescan() {
    rescanning = true
    rescanMessage = ''
    try {
      const s = await rescanRoots()
      serverName = s.serverName
      roots = s.roots
      rescanMessage = `Found ${s.roots.length} librar${s.roots.length === 1 ? 'y' : 'ies'}.`
      await session.refresh()
    } catch (e) {
      rescanMessage = e instanceof ApiError ? e.message : 'Rescan failed.'
    } finally {
      rescanning = false
    }
  }

  async function onChangePassword(e: SubmitEvent) {
    e.preventDefault()
    passwordError = ''

    if (!currentPassword) {
      passwordError = 'Enter your current password.'
      return
    }
    if (newPassword && newPassword !== confirmNewPassword) {
      passwordError = 'New passwords do not match.'
      return
    }
    if (!newUsername.trim() && !newPassword) {
      passwordError = 'Enter a new username and/or a new password.'
      return
    }

    changingPassword = true
    try {
      await changePassword(currentPassword, {
        newUsername: newUsername.trim() || undefined,
        newPassword: newPassword || undefined,
      })
      // Rotating the session secret invalidates this browser's own
      // cookie too — refreshing session state picks that up and the
      // app's top-level gate drops to the login screen on its own,
      // rather than the response itself failing confusingly mid-request.
      await session.refresh()
      router.navigate('/')
    } catch (e) {
      passwordError = e instanceof ApiError ? e.message : 'Could not change password.'
    } finally {
      changingPassword = false
    }
  }

  async function onLogout() {
    await logout().catch(() => {})
    await session.refresh()
    router.navigate('/')
  }
</script>

<div class="settings">
  <h1>Settings</h1>

  {#if loading}
    <p>Loading…</p>
  {:else if loadError}
    <p class="error">{loadError}</p>
  {:else}
    <form class="section" onsubmit={saveGeneral}>
      <h2>Server &amp; libraries</h2>

      <label>
        Server name
        <input type="text" bind:value={serverName} required />
      </label>

      {#if roots.length > 0}
        <div class="roots">
          {#each roots as root (root.id)}
            <div class="root-row">
              <span class="folder-name">{root.folderName}</span>
              <input type="text" bind:value={root.displayName} placeholder={root.folderName} />
              <label class="hidden-toggle">
                <input type="checkbox" bind:checked={root.hidden} />
                Hidden
              </label>
            </div>
          {/each}
        </div>
      {:else}
        <p class="hint">No media libraries found yet.</p>
      {/if}

      {#if generalMessage}<p class="success">{generalMessage}</p>{/if}
      {#if generalError}<p class="error">{generalError}</p>{/if}

      <div class="actions">
        <button type="submit" disabled={savingGeneral}>{savingGeneral ? 'Saving…' : 'Save'}</button>
        <button type="button" onclick={onRescan} disabled={rescanning}>
          {rescanning ? 'Rescanning…' : 'Rescan for new folders'}
        </button>
      </div>
      {#if rescanMessage}<p class="hint">{rescanMessage}</p>{/if}
    </form>

    <form class="section" onsubmit={onChangePassword}>
      <h2>Change username / password</h2>
      <p class="hint">Current username: <strong>{username}</strong></p>

      <label>
        Current password
        <input type="password" bind:value={currentPassword} autocomplete="current-password" required />
      </label>
      <label>
        New username <span class="optional">(optional)</span>
        <input type="text" bind:value={newUsername} autocomplete="username" />
      </label>
      <label>
        New password <span class="optional">(optional)</span>
        <input type="password" bind:value={newPassword} autocomplete="new-password" />
      </label>
      {#if newPassword}
        <label>
          Confirm new password
          <input type="password" bind:value={confirmNewPassword} autocomplete="new-password" />
        </label>
      {/if}

      {#if passwordError}<p class="error">{passwordError}</p>{/if}

      <div class="actions">
        <button type="submit" disabled={changingPassword}>
          {changingPassword ? 'Saving…' : 'Change'}
        </button>
      </div>
      <p class="hint">Changing this signs you (and every other open session) out.</p>
    </form>

    <div class="section">
      <button type="button" class="logout" onclick={onLogout}>Log out</button>
    </div>
  {/if}
</div>

<style>
  .settings {
    flex: 1;
    overflow-y: auto;
    padding: 24px 16px 64px;
    max-width: 520px;
    margin: 0 auto;
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 24px;
  }

  h1 {
    font-size: 1.3rem;
    margin: 0;
  }

  .section {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 16px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-elevated);
  }

  h2 {
    font-size: 1rem;
    margin: 0;
  }

  label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 0.85rem;
    color: var(--text-secondary);
  }

  .optional {
    color: var(--text-tertiary);
    font-weight: 400;
  }

  input[type='text'],
  input[type='password'] {
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
    font-size: 0.95rem;
  }

  .roots {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .root-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .folder-name {
    flex-shrink: 0;
    width: 110px;
    font-family: monospace;
    font-size: 0.8rem;
    color: var(--text-tertiary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .root-row input[type='text'] {
    flex: 1;
    min-width: 0;
  }

  .hidden-toggle {
    flex-direction: row;
    align-items: center;
    gap: 4px;
    flex-shrink: 0;
    font-size: 0.8rem;
  }

  .actions {
    display: flex;
    gap: 8px;
  }

  button {
    padding: 8px 14px;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
    font-size: 0.85rem;
    cursor: pointer;
  }

  button[type='submit'] {
    background: var(--accent);
    color: white;
    border: none;
    font-weight: 600;
  }

  button:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .logout {
    align-self: flex-start;
    color: var(--danger);
  }

  .hint {
    margin: 0;
    font-size: 0.8rem;
    color: var(--text-tertiary);
  }

  .success {
    margin: 0;
    color: var(--text-secondary);
    font-size: 0.85rem;
  }

  .error {
    margin: 0;
    color: var(--danger);
    font-size: 0.85rem;
  }
</style>
