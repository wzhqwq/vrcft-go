<script lang="ts">
  import {Trash2} from 'lucide-svelte'
  import {copy} from '../../../copy/zh-CN.js'
  import {Button, IconButton} from '../../components/ui/index.js'
  import ChannelSelectorDialog from './ChannelSelectorDialog.svelte'
  import {createRowKeys} from './row-keys.js'

  type Props = {
    values: readonly (readonly string[])[];
    id?: string;
    error?: string;
    onChange: (values: string[][]) => void;
  }

  let {values, id = 'mutual-exclusion', error, onChange}: Props = $props()
  const rowKeys = createRowKeys()

  function clone(values: readonly (readonly string[])[]): string[][] {
    return values.map((group) => [...group])
  }

  function update(index: number, group: string[]) {
    onChange(values.map((current, currentIndex) => currentIndex === index ? [...group] : [...current]))
  }

  function remove(index: number) {
    rowKeys.remove(index)
    onChange(values.filter((_, current) => current !== index).map((group) => [...group]))
  }

  function unavailable(index: number): Record<string, string> {
    return Object.fromEntries(values.flatMap((group, groupIndex) => groupIndex === index
      ? []
      : group.map((channel) => [channel, copy.format.channelInGroup(groupIndex)])))
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

      <ChannelSelectorDialog
        id={`${id}-${rowKeys.at(index)}-selector`}
        selected={group}
        triggerLabel={copy.format.editGroupChannels(index)}
        title={copy.format.selectGroupChannels(index)}
        description={copy.text.mutualHowItWorksDescription}
        emptyText={copy.text.chooseAtLeastTwoChannels}
        unavailable={unavailable(index)}
        onChange={(selection) => update(index, selection)}
      />
      {#if group.length === 1}<p class="text-sm text-warning">{copy.text.chooseOneMoreChannel}</p>{/if}
    </section>
  {/each}

  <Button label={copy.text.addGroup} tone="secondary" class="justify-self-start" onclick={() => onChange([...clone(values), []])} />
  {#if error}<p class="text-sm text-danger" id={`${id}-error`} role="alert">{error}</p>{/if}
</fieldset>
