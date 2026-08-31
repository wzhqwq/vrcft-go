<script lang="ts">
  import type {Snippet} from 'svelte';

  type Props = {
    label: string;
    description?: string;
    children: Snippet;
  };

  let {label, description, children}: Props = $props();
</script>

<div class="grid min-w-0 gap-2 border-t border-border pt-4">
  <div class="grid min-w-0 gap-1">
    <p class="min-w-0 break-words font-semibold text-text">{label}</p>
    {#if description}<p class="min-w-0 break-words text-sm text-text-muted">{description}</p>{/if}
  </div>
  <div class="min-w-0">{@render children()}</div>
</div>
