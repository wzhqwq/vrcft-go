<script lang="ts">
  import type {Snippet} from 'svelte';
  import {Collapsible as BitsCollapsible} from 'bits-ui';

  type Props = {
    triggerLabel: string;
    open?: boolean;
    disabled?: boolean;
    children?: Snippet;
  };

  let {triggerLabel, open = $bindable(false), disabled = false, children}: Props = $props();
</script>

<BitsCollapsible.Root bind:open {disabled} class="group min-w-0">
  <BitsCollapsible.Trigger class="focus-ring flex min-w-0 items-center justify-between gap-3 rounded-lg border border-border bg-surface px-3 py-2 text-left font-semibold text-text hover:bg-surface-raised disabled:cursor-not-allowed disabled:opacity-60">
    <span>{triggerLabel}</span>
    <span aria-hidden="true" class="transition-transform group-data-[state=open]:rotate-180">⌄</span>
  </BitsCollapsible.Trigger>
  <BitsCollapsible.Content class="min-w-0 overflow-hidden data-[state=closed]:hidden">
    <div class="pt-3">
      {#if children}{@render children()}{/if}
    </div>
  </BitsCollapsible.Content>
</BitsCollapsible.Root>
