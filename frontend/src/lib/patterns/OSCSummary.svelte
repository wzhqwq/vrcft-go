<script lang="ts">
  import {copy} from '../../copy/zh-CN.js';
  import {Badge} from '../components/ui/index.js';

  export interface OSCSummaryProps {
    state: 'not_running' | 'discovering' | 'discovered' | 'manual' | 'error';
    id?: string;
    host?: string;
    port?: number;
    error?: string;
  }

  const stateTones: Record<OSCSummaryProps['state'], 'neutral' | 'success' | 'warning' | 'danger'> = {
    not_running: 'warning',
    discovering: 'neutral',
    discovered: 'success',
    manual: 'neutral',
    error: 'danger',
  };

  const generatedId = globalThis.crypto?.randomUUID?.() ?? `osc-summary-${Math.random().toString(36).slice(2)}`;
  let {state, id = generatedId, host, port, error}: OSCSummaryProps = $props();
  let titleId = $derived(`${id}-title`);
  let target = $derived(host && port !== undefined ? `${host}:${port}` : copy.text.noTarget);
</script>

<section class="surface-card grid min-w-0 gap-3" aria-labelledby={titleId}>
  <div class="flex min-w-0 items-center justify-between gap-3">
    <h2 class="min-w-0 font-semibold text-text" id={titleId}>{copy.text.oscOutput}</h2>
    <Badge label={copy.state.osc[state].label} tone={stateTones[state]} />
  </div>
  <dl class="grid min-w-0 gap-1 text-sm">
    <dt class="text-text-muted">{copy.text.discovery}</dt>
    <dd class="text-text">{copy.state.osc[state].discovery}</dd>
    <dt class="mt-2 text-text-muted">{copy.text.outputTarget}</dt>
    <dd class="min-w-0 break-all text-text">{target}</dd>
  </dl>
  {#if state === 'error' && error}
    <p class="min-w-0 break-words text-sm text-danger" role="alert">{error}</p>
  {/if}
</section>
