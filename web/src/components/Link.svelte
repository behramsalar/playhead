<script lang="ts">
  import { router } from '../lib/router.svelte'

  interface Props {
    href: string
    class?: string
    children?: import('svelte').Snippet
    [key: string]: unknown
  }

  let { href, class: className, children, ...rest }: Props = $props()

  function onClick(e: MouseEvent) {
    if (e.defaultPrevented || e.button !== 0) return
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
    e.preventDefault()
    router.navigate(href)
  }
</script>

<a {href} class={className} onclick={onClick} {...rest}>
  {@render children?.()}
</a>
