<script lang="ts">
  import { login, ApiError } from '../api/client'
  import { session } from '../lib/session.svelte'

  let username = $state('')
  let password = $state('')
  let submitting = $state(false)
  let error = $state('')

  async function onSubmit(e: SubmitEvent) {
    e.preventDefault()
    error = ''
    submitting = true
    try {
      await login(username, password)
      await session.refresh()
    } catch (e) {
      error = e instanceof ApiError && e.status === 401 ? e.message : 'Could not log in. Is the server reachable?'
    } finally {
      submitting = false
    }
  }
</script>

<div class="login">
  <form onsubmit={onSubmit}>
    <h1>{session.serverName || 'Video Browser'}</h1>

    <label>
      Username
      <input type="text" bind:value={username} autocomplete="username" required />
    </label>

    <label>
      Password
      <input type="password" bind:value={password} autocomplete="current-password" required />
    </label>

    {#if error}
      <p class="error">{error}</p>
    {/if}

    <button type="submit" disabled={submitting}>{submitting ? 'Signing in…' : 'Sign in'}</button>
  </form>
</div>

<style>
  .login {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 16px;
  }

  form {
    width: 100%;
    max-width: 320px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  h1 {
    font-size: 1.2rem;
    margin: 0 0 4px;
    text-align: center;
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

  .error {
    margin: 0;
    color: var(--danger);
    font-size: 0.85rem;
    text-align: center;
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
