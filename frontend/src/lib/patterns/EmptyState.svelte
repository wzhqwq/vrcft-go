<script lang="ts">
  import {Button} from '../components/ui/index.js';

  type Props = {
    title: string;
    id?: string;
    description?: string;
    actionLabel?: string;
    onAction?: () => void;
  };

  const generatedId = globalThis.crypto?.randomUUID?.() ?? `empty-state-${Math.random().toString(36).slice(2)}`;
  let {title, id = generatedId, description, actionLabel, onAction}: Props = $props();
  let titleId = $derived(`${id}-title`);
</script>

<section class="surface-card grid min-w-0 place-items-center gap-3 px-6 py-10 text-center" aria-labelledby={titleId}>
  <div class="grid min-w-0 gap-1">
    <h2 class="min-w-0 break-words text-lg font-semibold text-text" id={titleId}>{title}</h2>
    {#if description}<p class="min-w-0 break-words text-sm text-text-muted">{description}</p>{/if}
  </div>
  {#if actionLabel && onAction}<Button label={actionLabel} tone="secondary" onclick={onAction} />{/if}
</section>
