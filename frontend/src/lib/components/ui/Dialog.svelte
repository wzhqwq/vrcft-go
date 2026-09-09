<script lang="ts">
  import type {Snippet} from 'svelte';
  import {Dialog as BitsDialog} from 'bits-ui';

  type Props = {
    triggerLabel: string;
    title: string;
    description?: string;
    closeLabel?: string;
    open?: boolean;
    showTrigger?: boolean;
    children?: Snippet;
  };

  let {
    triggerLabel,
    title,
    description,
    closeLabel = '关闭',
    open = $bindable(false),
    showTrigger = true,
    children,
  }: Props = $props();
</script>

<BitsDialog.Root bind:open>
  {#if showTrigger}
    <BitsDialog.Trigger class="focus-ring inline-flex min-w-0 items-center justify-center rounded-lg border border-border bg-surface-raised px-4 py-2 font-semibold text-text transition-colors hover:bg-surface">
      {triggerLabel}
    </BitsDialog.Trigger>
  {/if}
  <BitsDialog.Portal>
    <BitsDialog.Overlay class="fixed inset-0 z-40 bg-overlay/65 data-[state=closed]:opacity-0 data-[state=open]:opacity-100 motion-safe:transition-opacity" />
    <BitsDialog.Content class="fixed left-1/2 top-1/2 z-50 grid max-h-[calc(100dvh-2rem)] w-[min(32rem,calc(100%-2rem))] min-w-0 -translate-x-1/2 -translate-y-1/2 gap-4 overflow-y-auto rounded-xl border border-border bg-surface p-5 text-text shadow-xl shadow-shadow/40 data-[state=closed]:opacity-0 data-[state=open]:opacity-100 motion-safe:transition-opacity">
      <div class="grid min-w-0 gap-1">
        <BitsDialog.Title level={2} class="text-lg font-bold">{title}</BitsDialog.Title>
        {#if description}<BitsDialog.Description class="text-text-muted">{description}</BitsDialog.Description>{/if}
      </div>
      {#if children}{@render children()}{/if}
      <div class="flex justify-end">
        <BitsDialog.Close class="focus-ring rounded-lg border border-border bg-surface-raised px-4 py-2 font-semibold text-text transition-colors hover:bg-surface">{closeLabel}</BitsDialog.Close>
      </div>
    </BitsDialog.Content>
  </BitsDialog.Portal>
</BitsDialog.Root>
