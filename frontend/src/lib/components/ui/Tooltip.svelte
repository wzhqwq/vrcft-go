<script lang="ts">
  import type {Snippet} from 'svelte';
  import {Tooltip as BitsTooltip} from 'bits-ui';

  type DefaultTrigger = {
    triggerLabel: string;
    trigger?: never;
  };
  type CustomTrigger = {
    trigger: Snippet<[Record<string, unknown>]>;
    triggerLabel?: never;
  };
  type Props = {
    content: string;
    delayDuration?: number;
    disabled?: boolean;
  } & (DefaultTrigger | CustomTrigger);

  function assertTrigger(trigger: CustomTrigger['trigger'] | undefined, triggerLabel: string | undefined): void {
    if (!trigger && !triggerLabel?.trim()) {
      throw new Error('Tooltip requires a non-empty triggerLabel or trigger snippet');
    }
  }

  let {triggerLabel, content, delayDuration = 700, disabled = false, trigger}: Props = $props();
  $effect.pre(() => assertTrigger(trigger, triggerLabel));
</script>

<BitsTooltip.Provider>
  <BitsTooltip.Root {delayDuration} {disabled}>
    {#if trigger}
      <BitsTooltip.Trigger>
        {#snippet child({props})}
          {@render trigger(props)}
        {/snippet}
      </BitsTooltip.Trigger>
    {:else}
      <BitsTooltip.Trigger class="focus-ring inline-flex min-w-0 items-center justify-center rounded-lg border border-border bg-surface-raised px-3 py-2 text-text hover:bg-surface">
        {triggerLabel}
      </BitsTooltip.Trigger>
    {/if}
    <BitsTooltip.Portal>
      <BitsTooltip.Content sideOffset={6} class="z-50 max-w-64 rounded-md border border-border bg-surface-raised px-3 py-2 text-sm text-text shadow-lg shadow-shadow/30" role="tooltip">
        {content}
        <BitsTooltip.Arrow class="fill-surface-raised" />
      </BitsTooltip.Content>
    </BitsTooltip.Portal>
  </BitsTooltip.Root>
</BitsTooltip.Provider>
