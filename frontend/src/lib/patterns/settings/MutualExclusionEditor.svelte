<script lang="ts">
  import {Trash2, X} from 'lucide-svelte'
  import {copy} from '../../../copy/zh-CN.js'
  import {Button, IconButton} from '../../components/ui/index.js'
  import ChannelSelector from './ChannelSelector.svelte'
  import {channelLabel} from './channels.js'
  import {createRowKeys} from './row-keys.js'

  type Props = {
    values: readonly (readonly string[])[];
    id?: string;
    error?: string;
    onChange: (values: string[][]) => void;
  }

  let {values, id = 'mutual-exclusion', error, onChange}: Props = $props()
  const rowKeys = createRowKeys()
  let expanded = $state<Record<number, boolean>>({})

  function clone(values: readonly (readonly string[])[]): string[][] {
    return values.map((group) => [...group])
  }

  function update(index: number, group: string[]) {
    onChange(values.map((current, currentIndex) => currentIndex === index ? [...group] : [...current]))
  }

  function removeMember(index: number, channel: string) {
    update(index, values[index].filter((current) => current !== channel))
  }

  function remove(index: number) {
    const key = rowKeys.at(index)
    delete expanded[key]
    rowKeys.remove(index)
    onChange(values.filter((_, current) => current !== index).map((group) => [...group]))
  }

  function unavailable(index: number): Record<string, string> {
    return Object.fromEntries(values.flatMap((group, groupIndex) => groupIndex === index
      ? []
      : group.map((channel) => [channel, copy.format.channelInGroup(groupIndex)])))
  }

  function isExpanded(index: number): boolean {
    return expanded[rowKeys.at(index)] ?? values[index].length === 0
  }

  function toggle(index: number) {
    const key = rowKeys.at(index)
    expanded[key] = !isExpanded(index)
  }
</script>

<fieldset class="grid min-w-0 gap-4" id={id} tabindex="-1" aria-describedby={error ? `${id}-error` : undefined}>
  <legend class="sr-only">{copy.text.mutualExclusion}</legend>
  <div class="grid min-w-0 gap-1">
    <p class="font-semibold text-text">{copy.text.mutualHowItWorks}</p>
    <p class="text-sm text-text-muted">{copy.text.mutualHowItWorksDescription}</p>
  </div>

  {#if values.length === 0}
    <div class="grid min-w-0 gap-1 rounded-lg border border-dashed border-border p-4">
      <p class="font-semibold text-text">{copy.text.noMutualGroups}</p>
      <p class="text-sm text-text-muted">{copy.text.noMutualGroupsDescription}</p>
    </div>
  {/if}

  {#each values as group, index (rowKeys.at(index))}
    <section class="grid min-w-0 gap-3 rounded-lg border border-border bg-surface p-4" role="group" aria-label={copy.format.group(index)}>
      <div class="flex min-w-0 items-start justify-between gap-3">
        <div class="grid min-w-0 gap-1">
          <h3 class="font-semibold text-text">{copy.format.group(index)}</h3>
          <p class="text-sm text-text-muted">{copy.text.mutualGroupBehavior}</p>
        </div>
        <IconButton label={copy.format.removeGroup(index)} title={copy.format.removeGroup(index)} onclick={() => remove(index)}>
          <Trash2 aria-hidden="true" class="size-4" />
        </IconButton>
      </div>

      <div class="flex min-w-0 flex-wrap gap-2" aria-label={copy.format.selectedGroupChannels(index)}>
        {#each group as channel (channel)}
          <button
            type="button"
            class="focus-ring inline-flex min-w-0 items-center gap-1.5 rounded-full border border-accent bg-accent/15 px-3 py-1.5 text-sm text-text"
            aria-label={copy.format.removeChannelFromGroup(index, channelLabel(channel))}
            title={channel}
            onclick={() => removeMember(index, channel)}
          >
            <span class="min-w-0 truncate">{channelLabel(channel)}</span>
            <X aria-hidden="true" class="size-3.5 shrink-0" />
          </button>
        {:else}
          <p class="text-sm text-text-muted">{copy.text.chooseAtLeastTwoChannels}</p>
        {/each}
      </div>
      {#if group.length === 1}<p class="text-sm text-warning">{copy.text.chooseOneMoreChannel}</p>{/if}

      <Button label={copy.format.editGroupChannels(index)} tone="secondary" class="justify-self-start" aria-expanded={isExpanded(index)} onclick={() => toggle(index)} />
      {#if isExpanded(index)}
        <ChannelSelector
          id={`${id}-${rowKeys.at(index)}-selector`}
          selected={group}
          unavailable={unavailable(index)}
          onChange={(selection) => update(index, selection)}
        />
      {/if}
    </section>
  {/each}

  <Button label={copy.text.addGroup} tone="secondary" class="justify-self-start" onclick={() => onChange([...clone(values), []])} />
  {#if error}<p class="text-sm text-danger" id={`${id}-error`} role="alert">{error}</p>{/if}
</fieldset>
