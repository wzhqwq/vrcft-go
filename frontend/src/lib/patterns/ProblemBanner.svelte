<script lang="ts">
  import {Button} from '../components/ui/index.js';
  import type {StatusTone} from './StatusCard.svelte';

  type Props = {
    title: string;
    detail: string;
    tone: Extract<StatusTone, 'warning' | 'danger'>;
    diagnosticCode?: string;
    onCopyDiagnostic?: (diagnostic: string) => void;
  };

  let {title, detail, tone, diagnosticCode, onCopyDiagnostic}: Props = $props();
  let safeCode = $derived(diagnosticCode && /^[a-z0-9_-]+$/.test(diagnosticCode) ? diagnosticCode : 'unknown');
  let safeDiagnostic = $derived(`问题代码：${safeCode}`);
</script>

<section class={`min-w-0 rounded-xl border p-4 ${tone === 'danger' ? 'border-danger bg-danger/10' : 'border-warning bg-warning/10'}`} role="alert" aria-labelledby="problem-banner-title">
  <div class="flex min-w-0 flex-wrap items-start justify-between gap-3">
    <div class="grid min-w-0 gap-1">
      <h2 class="min-w-0 break-words font-semibold text-text" id="problem-banner-title">{title}</h2>
      <p class="min-w-0 break-words text-sm text-text-muted">{detail}</p>
    </div>
    {#if diagnosticCode}
      <Button label="复制诊断信息" tone="secondary" onclick={() => onCopyDiagnostic?.(safeDiagnostic)} />
    {/if}
  </div>
</section>
