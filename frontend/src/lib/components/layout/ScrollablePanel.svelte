<script lang="ts">
  import type {Snippet} from 'svelte';

  type Props = {
    children: Snippet;
    class?: string;
  };

  let {children, class: className = ''}: Props = $props();
</script>

<section class={`min-h-0 min-w-0 overflow-auto ${className}`}>{@render children()}</section>
