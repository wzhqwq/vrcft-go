<script lang="ts">
  import {Badge} from '../components/ui/index.js';

  export interface OSCSummaryProps {
    state: 'not_running' | 'discovering' | 'discovered' | 'manual' | 'error';
    host?: string;
    port?: number;
    error?: string;
  }

  const stateLabels: Record<OSCSummaryProps['state'], string> = {
    not_running: 'OSC 未运行',
    discovering: '正在发现 OSC 输出',
    discovered: '已发现 OSC 输出',
    manual: '正在使用手动 OSC 输出',
    error: 'OSC 输出错误',
  };
  const stateTones: Record<OSCSummaryProps['state'], 'neutral' | 'success' | 'warning' | 'danger'> = {
    not_running: 'warning',
    discovering: 'neutral',
    discovered: 'success',
    manual: 'neutral',
    error: 'danger',
  };

  let {state, host, port, error}: OSCSummaryProps = $props();
  let target = $derived(host && port !== undefined ? `${host}:${port}` : '未设置输出目标');
</script>

<section class="surface-card grid min-w-0 gap-3" aria-labelledby="osc-summary-title">
  <div class="flex min-w-0 items-center justify-between gap-3">
    <h2 class="min-w-0 font-semibold text-text" id="osc-summary-title">OSC 输出</h2>
    <Badge label={stateLabels[state]} tone={stateTones[state]} />
  </div>
  <dl class="grid min-w-0 gap-1 text-sm">
    <dt class="mt-2 text-text-muted">输出目标</dt>
    <dd class="min-w-0 break-all text-text">{target}</dd>
  </dl>
  {#if state === 'error' && error}
    <p class="min-w-0 break-words text-sm text-danger" role="alert">{error}</p>
  {/if}
</section>
