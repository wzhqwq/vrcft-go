<script lang="ts">
  import type {Snippet} from 'svelte';

  type Props = {
    title: string;
    description?: string;
    actions?: Snippet;
    class?: string;
  };

  let {title, description, actions, class: className = ''}: Props = $props();
</script>

<header class={`flex min-w-0 flex-wrap items-start justify-between gap-3 ${className}`}>
  <div class="min-w-0">
    <h1 class="text-xl font-bold text-text">{title}</h1>
    {#if description}<p class="mt-1 text-sm text-text-muted">{description}</p>{/if}
  </div>
  {#if actions}<div class="flex min-w-0 flex-wrap items-center gap-2">{@render actions()}</div>{/if}
</header>
