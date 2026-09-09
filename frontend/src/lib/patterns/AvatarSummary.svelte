<script lang="ts">
  import {copy} from '../../copy/zh-CN.js'
  import {Button, Spinner} from '../components/ui/index.js';

  type Props = {
    name: string;
    id: string;
    summaryId?: string;
    loading?: boolean;
    error?: string;
    onCopyId?: (id: string) => void;
  };

  const generatedSummaryId = globalThis.crypto?.randomUUID?.() ?? `avatar-summary-${Math.random().toString(36).slice(2)}`;
  let {name, id, summaryId = generatedSummaryId, loading = false, error, onCopyId}: Props = $props();
  let titleId = $derived(`${summaryId}-title`);
</script>

<section class="surface-card grid min-w-0 gap-3" aria-labelledby={titleId}>
  <div class="flex min-w-0 items-center justify-between gap-3">
    <h2 class="min-w-0 font-semibold text-text" id={titleId}>{copy.text.currentAvatar}</h2>
    <Button label={copy.text.copyAvatar} tone="secondary" onclick={() => onCopyId?.(id)} />
  </div>
  {#if loading}
    <Spinner label={copy.text.readAvatar} />
  {:else if error}
    <p class="text-sm text-danger" role="alert">{error}</p>
  {:else}
    <dl class="grid min-w-0 gap-2 text-sm">
      <div class="grid min-w-0 gap-1">
        <dt class="text-text-muted">{copy.text.name}</dt>
        <dd class="min-w-0 break-all text-text">{name || id || copy.text.notProvided}</dd>
      </div>
      <div class="grid min-w-0 gap-1">
        <dt class="text-text-muted">Avatar ID</dt>
        <dd class="min-w-0 break-all text-text">{id}</dd>
      </div>
    </dl>
  {/if}
</section>
