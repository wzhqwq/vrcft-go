<script lang="ts">
  import {Button} from '../components/ui/index.js';

  type Props = {
    title: string;
    description?: string;
    actionLabel?: string;
    onAction?: () => void;
  };

  let {title, description, actionLabel, onAction}: Props = $props();
</script>

<section class="surface-card grid min-w-0 place-items-center gap-3 px-6 py-10 text-center" aria-labelledby="empty-state-title">
  <div class="grid min-w-0 gap-1">
    <h2 class="min-w-0 break-words text-lg font-semibold text-text" id="empty-state-title">{title}</h2>
    {#if description}<p class="min-w-0 break-words text-sm text-text-muted">{description}</p>{/if}
  </div>
  {#if actionLabel && onAction}<Button label={actionLabel} tone="secondary" onclick={onAction} />{/if}
</section>
