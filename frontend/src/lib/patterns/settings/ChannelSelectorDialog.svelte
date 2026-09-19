<script lang="ts">
  import {Pencil} from 'lucide-svelte'
  import {copy} from '../../../copy/zh-CN.js'
  import {Dialog} from '../../components/ui/index.js'
  import ChannelSelector from './ChannelSelector.svelte'
  import {channelLabel} from './channels.js'

  type Props = {
    selected: readonly string[];
    triggerLabel: string;
    title: string;
    description?: string;
    emptyText?: string;
    mode?: 'single' | 'multiple';
    unavailable?: Readonly<Record<string, string>>;
    id?: string;
    onChange: (selected: string[]) => void;
  }

  let {
    selected,
    triggerLabel,
    title,
    description,
    emptyText = copy.text.noChannelsSelected,
    mode = 'multiple',
    unavailable = {},
    id = 'channel-selector-dialog',
    onChange,
  }: Props = $props()
  let open = $state(false)

  function change(selection: string[]) {
    onChange(selection)
    if (mode === 'single') open = false
  }
</script>

<Dialog
  bind:open
  {triggerLabel}
  {title}
  {description}
  closeLabel={copy.actions.done}
  triggerClass="focus-ring flex w-full min-w-0 items-center justify-between gap-3 rounded-lg border border-border bg-surface-raised p-3 text-left text-text transition-colors hover:bg-surface"
>
  {#snippet trigger()}
    <span class="flex min-w-0 flex-1 flex-wrap gap-2">
      {#each selected as channel (channel)}
        <span class="min-w-0 truncate rounded-full border border-accent bg-accent/15 px-3 py-1.5 text-sm font-normal" title={channel}>{channelLabel(channel)}</span>
      {:else}
        <span class="text-sm font-normal text-text-muted">{emptyText}</span>
      {/each}
    </span>
    <Pencil aria-hidden="true" class="size-4 shrink-0 text-text-muted" />
  {/snippet}
  {#snippet children()}
    <ChannelSelector {id} {selected} {mode} {unavailable} onChange={change} />
  {/snippet}
</Dialog>
