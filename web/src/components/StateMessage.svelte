<script lang="ts">
  interface Props {
    title: string
    detail?: string
    children?: import('svelte').Snippet
  }

  let { title, detail, children }: Props = $props()
</script>

<div class="state">
  <p class="title">{title}</p>
  {#if detail}
    <p class="detail">{detail}</p>
  {/if}
  {@render children?.()}
</div>

<style>
  .state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    text-align: center;
    padding: 64px 24px;
    color: var(--text-secondary);
  }

  .title {
    font-size: 1rem;
    font-weight: 600;
    color: var(--text);
    margin: 0;
  }

  .detail {
    font-size: 0.9rem;
    margin: 0;
    max-width: 40ch;
  }
</style>
