<script lang="ts">
  import type {Snippet} from 'svelte';

  type Tone = 'primary' | 'secondary' | 'danger';
  type Props = {
    label: string;
    tone?: Tone;
    type?: 'button' | 'submit' | 'reset';
    disabled?: boolean;
    loading?: boolean;
    loadingLabel?: string;
    children?: Snippet;
  };

  const toneClasses: Record<Tone, string> = {
    primary: 'border-accent bg-accent text-canvas hover:bg-accent/85',
    secondary: 'border-border bg-surface-raised text-text hover:bg-surface',
    danger: 'border-danger bg-danger text-canvas hover:bg-danger/85',
  };

  let {
    label,
    tone = 'primary',
    type = 'button',
    disabled = false,
    loading = false,
    loadingLabel = '正在处理',
    children,
  }: Props = $props();
  const statusId = globalThis.crypto?.randomUUID?.() ?? `button-status-${Math.random().toString(36).slice(2)}`;
</script>

<button
  {type}
  class={`focus-ring inline-flex min-w-0 items-center justify-center gap-2 rounded-lg border px-4 py-2 font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-60 ${toneClasses[tone]}`}
  aria-busy={loading || undefined}
  aria-describedby={loading ? statusId : undefined}
  disabled={disabled || loading}
>
  {#if loading}
    <span aria-hidden="true" class="size-4 animate-spin rounded-full border-2 border-current border-r-transparent"></span>
  {/if}
  {#if children}
    {@render children()}
  {:else}
    {label}
  {/if}
</button>
{#if loading}<span class="sr-only" id={statusId} role="status">{loadingLabel}</span>{/if}
