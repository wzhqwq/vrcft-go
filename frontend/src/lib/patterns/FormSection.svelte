<script lang="ts">
  import type {Snippet} from 'svelte';

  type Props = {
    title: string;
    description?: string;
    children: Snippet;
  };

  let {title, description, children}: Props = $props();
</script>

<section class="surface-card grid min-w-0 gap-4" aria-labelledby={`form-section-${title}`}>
  <header class="grid min-w-0 gap-1">
    <h2 class="min-w-0 break-words text-lg font-semibold text-text" id={`form-section-${title}`}>{title}</h2>
    {#if description}<p class="min-w-0 break-words text-sm text-text-muted">{description}</p>{/if}
  </header>
  <div class="grid min-w-0 gap-4">{@render children()}</div>
</section>
