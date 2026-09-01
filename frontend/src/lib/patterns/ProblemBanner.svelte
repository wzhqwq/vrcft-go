<script lang="ts">
  import {Button} from '../components/ui/index.js';
  import type {StatusTone} from './StatusCard.svelte';

  type Props = {
    title: string;
    detail: string;
    tone: Extract<StatusTone, 'warning' | 'danger'>;
    id?: string;
    diagnosticCode?: string;
    onCopyDiagnostic?: (diagnostic: string) => void;
  };

  const generatedId = globalThis.crypto?.randomUUID?.() ?? `problem-banner-${Math.random().toString(36).slice(2)}`;
  let {title, detail, tone, id = generatedId, diagnosticCode, onCopyDiagnostic}: Props = $props();
  let titleId = $derived(`${id}-title`);
  let safeCode = $derived(diagnosticCode && /^[a-z0-9_-]{1,64}$/.test(diagnosticCode) ? diagnosticCode : 'unknown');
  let safeDiagnostic = $derived(`问题代码：${safeCode}`);
</script>

<section class={`min-w-0 rounded-xl border p-4 ${tone === 'danger' ? 'border-danger bg-danger/10' : 'border-warning bg-warning/10'}`} role="alert" aria-labelledby={titleId}>
  <div class="flex min-w-0 flex-wrap items-start justify-between gap-3">
    <div class="grid min-w-0 gap-1">
      <h2 class="min-w-0 break-words font-semibold text-text" id={titleId}>{title}</h2>
      <p class="min-w-0 break-words text-sm text-text-muted">{detail}</p>
    </div>
    {#if diagnosticCode}
      <Button label="复制诊断信息" tone="secondary" onclick={() => onCopyDiagnostic?.(safeDiagnostic)} />
    {/if}
  </div>
</section>
