<script lang="ts">
  import {copy} from '../../../copy/zh-CN.js'
  import {TextField} from '../../components/ui/index.js'
  import {channelGroups} from './channels.js'

  type Props = {
    selected: readonly string[];
    mode?: 'single' | 'multiple';
    unavailable?: Readonly<Record<string, string>>;
    id?: string;
    onChange: (selected: string[]) => void;
  }

  let {selected, mode = 'multiple', unavailable = {}, id = 'channel-selector', onChange}: Props = $props()
  let query = $state('')
  let normalizedQuery = $derived(query.trim().toLocaleLowerCase())
  let groups = $derived(channelGroups.map((group) => ({
    ...group,
    channels: group.channels.filter((channel) => !normalizedQuery
      || channel.label.toLocaleLowerCase().includes(normalizedQuery)
      || channel.id.toLocaleLowerCase().includes(normalizedQuery)),
  })).filter((group) => group.channels.length > 0))

  function toggle(channel: string) {
    if (mode === 'single') {
      if (!selected.includes(channel)) onChange([channel])
      return
    }
    onChange(selected.includes(channel)
      ? selected.filter((current) => current !== channel)
      : [...selected, channel])
  }
</script>

<div class="grid min-w-0 gap-3 rounded-lg border border-border bg-surface p-3" id={id}>
  <TextField type="search" label={copy.text.searchChannels} bind:value={query} placeholder={copy.text.searchChannelsPlaceholder} />
  <div class="grid max-h-80 min-w-0 gap-4 overflow-y-auto pr-1">
    {#each groups as group (group.category)}
      <section class="grid min-w-0 gap-2" aria-labelledby={`${id}-${group.category}`}>
        <h4 class="text-sm font-semibold text-text" id={`${id}-${group.category}`}>{group.category}</h4>
        <div class="flex min-w-0 flex-wrap gap-2">
          {#each group.channels as channel (channel.id)}
            {@const selectedChannel = selected.includes(channel.id)}
            {@const reason = selectedChannel ? undefined : unavailable[channel.id]}
            {@const reasonId = `${id}-${channel.id.replace(/[^a-zA-Z0-9_-]/g, '-')}-reason`}
            <button
              type="button"
              class="focus-ring rounded-full border px-3 py-1.5 text-sm transition-colors aria-pressed:border-accent aria-pressed:bg-accent/15 aria-pressed:text-text disabled:cursor-not-allowed disabled:opacity-45"
              class:border-border={!selectedChannel}
              class:bg-surface-raised={!selectedChannel}
              class:text-text-muted={!selectedChannel}
              aria-pressed={selectedChannel}
              aria-describedby={reason ? reasonId : undefined}
              disabled={Boolean(reason)}
              title={channel.id}
              onclick={() => toggle(channel.id)}
            >{channel.label}</button>
            {#if reason}<span class="sr-only" id={reasonId}>{reason}</span>{/if}
          {/each}
        </div>
      </section>
    {:else}
      <p class="text-sm text-text-muted">{copy.text.noMatchingChannels}</p>
    {/each}
  </div>
</div>
