<script lang="ts">
  import {Plus, Trash2} from 'lucide-svelte'
  import {copy} from '../../../copy/zh-CN.js'
  import {IconButton, SelectField, TextField} from '../../components/ui/index.js'
  import type {ProcessingChannel, ProcessingOverride} from '../../modules/settings/form.js'
  import {createRowKeys} from './row-keys.js'
  import ProcessingChannelFields from './ProcessingChannelFields.svelte'

  type Props = {
    defaultChannel: Readonly<ProcessingChannel>;
    values: readonly Readonly<ProcessingOverride>[];
    id?: string;
    defaultId?: string;
    error?: string;
    onDefaultChange: (value: ProcessingChannel) => void;
    onChange: (values: ProcessingOverride[]) => void;
  }

  let {
    defaultChannel,
    values,
    id = 'channel-overrides',
    defaultId = 'default-channel',
    error,
    onDefaultChange,
    onChange,
  }: Props = $props()
  const rowKeys = createRowKeys()
  let selected = $state('default')
  let options = $derived([
    {value: 'default', label: copy.text.defaultProcessing},
    ...values.map((override, index) => ({
      value: `override:${index}`,
      label: override.name.trim() ? copy.format.customProcessing(override.name.trim()) : copy.format.unnamedCustomProcessing(index),
    })),
  ])
  let selectedIndex = $derived(selected.startsWith('override:') ? Number(selected.slice('override:'.length)) : -1)
  let selectedOverride = $derived(selectedIndex >= 0 ? values[selectedIndex] : undefined)

  $effect(() => {
    if (selected !== 'default' && selectedOverride === undefined) selected = 'default'
  })

  function cloneChannel(channel: Readonly<ProcessingChannel>): ProcessingChannel {
    return {
      calibration: {...channel.calibration},
      tuning: {...channel.tuning},
      filter: {...channel.filter},
      dropout: {...channel.dropout},
    }
  }

  function cloneOverride(value: Readonly<ProcessingOverride>): ProcessingOverride {
    return {name: value.name, channel: cloneChannel(value.channel)}
  }

  function add() {
    const index = values.length
    onChange([...values.map(cloneOverride), {name: '', channel: cloneChannel(defaultChannel)}])
    selected = `override:${index}`
  }

  function remove() {
    if (selectedIndex < 0 || selectedOverride === undefined) return
    const index = selectedIndex
    selected = 'default'
    rowKeys.remove(index)
    onChange(values.filter((_, current) => current !== index).map(cloneOverride))
  }

  function updateSelected(next: ProcessingOverride) {
    if (selectedIndex < 0) return
    onChange(values.map((value, index) => index === selectedIndex ? cloneOverride(next) : cloneOverride(value)))
  }
</script>

<fieldset class="grid min-w-0 gap-4" id={id} tabindex="-1" aria-describedby={error ? `${id}-error` : undefined}>
  <legend class="sr-only">{copy.text.processingProfiles}</legend>
  <div class="flex min-w-0 items-end gap-2">
    <div class="min-w-0 flex-1">
      <SelectField label={copy.text.processingProfile} bind:value={selected} {options} />
    </div>
    <IconButton label={copy.text.addCustomProcessing} title={copy.text.addCustomProcessing} onclick={add}>
      <Plus aria-hidden="true" class="size-4" />
    </IconButton>
    {#if selectedOverride}
      <IconButton label={copy.text.removeCustomProcessing} title={copy.text.removeCustomProcessing} onclick={remove}>
        <Trash2 aria-hidden="true" class="size-4" />
      </IconButton>
    {/if}
  </div>

  {#if selectedOverride}
    <TextField
      id={`${id}-${rowKeys.at(selectedIndex)}-name`}
      label={copy.text.overrideChannelName}
      description={copy.text.overrideChannelNameDescription}
      value={selectedOverride.name}
      oninput={(event) => updateSelected({...cloneOverride(selectedOverride), name: event.currentTarget.value})}
    />
    <ProcessingChannelFields
      id={`${id}-${rowKeys.at(selectedIndex)}-channel`}
      value={selectedOverride.channel}
      onChange={(channel) => updateSelected({...cloneOverride(selectedOverride), channel})}
    />
  {:else}
    <ProcessingChannelFields id={defaultId} value={defaultChannel} onChange={onDefaultChange} />
  {/if}

  {#if error}<p class="text-sm text-danger" id={`${id}-error`} role="alert">{error}</p>{/if}
</fieldset>
