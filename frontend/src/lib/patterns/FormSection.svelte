<script lang="ts">
  import type {Snippet} from 'svelte';

  type Props = {
    title: string;
    id?: string;
    description?: string;
    children: Snippet;
  };

  const generatedId = globalThis.crypto?.randomUUID?.() ?? `form-section-${Math.random().toString(36).slice(2)}`;
  let {title, id = generatedId, description, children}: Props = $props();
  let titleId = $derived(`${id}-title`);
</script>

<section class="surface-card grid min-w-0 gap-4" aria-labelledby={titleId}>
  <header class="grid min-w-0 gap-1">
    <h2 class="min-w-0 break-words text-lg font-semibold text-text" id={titleId}>{title}</h2>
    {#if description}<p class="min-w-0 break-words text-sm text-text-muted">{description}</p>{/if}
  </header>
  <div class="grid min-w-0 gap-4">{@render children()}</div>
</section>
