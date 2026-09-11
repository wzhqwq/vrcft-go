<script lang="ts">
  export interface DetailItem {
    label: string;
    value: string;
  }

  type Props = {items: DetailItem[]};
  let {items}: Props = $props();
</script>

<dl class="grid min-w-0 gap-2">
  {#each items as item}
    <div class="grid min-w-0 grid-cols-[minmax(5rem,1fr)_minmax(0,2fr)] gap-2 border-b border-border pb-2 last:border-b-0 last:pb-0">
      <dt class="min-w-0 break-words text-sm text-text-muted">{item.label}</dt>
      <dd class="min-w-0 break-words text-sm text-text">{item.value}</dd>
    </div>
  {/each}
</dl>
