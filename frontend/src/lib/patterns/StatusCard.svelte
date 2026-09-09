<script lang="ts">
  import {copy} from '../../copy/zh-CN.js'
  import {Badge, Spinner} from '../components/ui/index.js';

  export type StatusTone = 'neutral' | 'success' | 'warning' | 'danger';

  type Props = {
    title: string;
    detail?: string;
    tone?: StatusTone;
    label?: string;
    loading?: boolean;
    loadingLabel?: string;
  };

  let {title, detail, label, tone = 'neutral', loading = false, loadingLabel = copy.state.loading}: Props = $props();
</script>

<article class="surface-card grid min-w-0 gap-3" aria-label={title}>
  <div class="flex min-w-0 items-start justify-between gap-3">
    <h3 class="min-w-0 font-semibold text-text">{title}</h3>
    {#if label}<Badge {label} {tone} />{/if}
  </div>
  {#if loading}
    <Spinner label={loadingLabel} />
  {:else if detail}
    <p class="min-w-0 break-words text-sm text-text-muted">{detail}</p>
  {/if}
</article>
