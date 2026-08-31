<script lang="ts">
  import {Tooltip as BitsTooltip} from 'bits-ui';

  type Props = {
    triggerLabel: string;
    content: string;
    delayDuration?: number;
    disabled?: boolean;
  };

  let {triggerLabel, content, delayDuration = 700, disabled = false}: Props = $props();
</script>

<BitsTooltip.Provider>
  <BitsTooltip.Root {delayDuration} {disabled}>
    <BitsTooltip.Trigger class="focus-ring inline-flex min-w-0 items-center justify-center rounded-lg border border-border bg-surface-raised px-3 py-2 text-text hover:bg-surface">
      {triggerLabel}
    </BitsTooltip.Trigger>
    <BitsTooltip.Portal>
      <BitsTooltip.Content sideOffset={6} class="z-50 max-w-64 rounded-md border border-border bg-surface-raised px-3 py-2 text-sm text-text shadow-lg shadow-black/30" role="tooltip">
        {content}
        <BitsTooltip.Arrow class="fill-surface-raised" />
      </BitsTooltip.Content>
    </BitsTooltip.Portal>
  </BitsTooltip.Root>
</BitsTooltip.Provider>
