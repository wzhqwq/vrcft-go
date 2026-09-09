<script lang="ts">
  import {copy} from '../../../copy/zh-CN.js'
  import {Button, Collapsible, TextField} from '../../components/ui/index.js'
  import type {ProcessingChannel, ProcessingOverride} from '../../modules/settings/form.js'
  import {createRowKeys} from './row-keys.js'
  import ProcessingChannelFields from './ProcessingChannelFields.svelte'

  type Props = {
    values: readonly Readonly<ProcessingOverride>[];
    id?: string;
    error?: string;
    onChange: (values: ProcessingOverride[]) => void;
  }

  const emptyChannel: ProcessingChannel = {
    calibration: {enabled: false, neutral: 0, min: 0, max: 1, gain: 1, invert: false},
    tuning: {deadzone: 0, gain: 1, exponent: 1, clampEnabled: false, clampMin: 0, clampMax: 1},
    filter: {mode: 'ema', emaAlpha: 0.5, minCutoff: 1, beta: 0, derivativeCutoff: 1},
    dropout: {holdDurationMs: 0, decayDurationMs: 0, staleAfterMs: 0},
  }

  let {values, id = 'channel-overrides', error, onChange}: Props = $props()
  const rowKeys = createRowKeys()

  function remove(index: number) {
    rowKeys.remove(index)
    onChange(values.filter((_, current) => current !== index).map(cloneOverride))
  }

  function cloneOverride(value: Readonly<ProcessingOverride>): ProcessingOverride {
    return {name: value.name, channel: {
      calibration: {...value.channel.calibration}, tuning: {...value.channel.tuning},
      filter: {...value.channel.filter}, dropout: {...value.channel.dropout},
    }}
  }

  function update(index: number, next: ProcessingOverride) {
    onChange(values.map((value, current) => current === index ? cloneOverride(next) : cloneOverride(value)))
  }
</script>

<fieldset class="grid min-w-0 gap-3" id={id} tabindex="-1" aria-describedby={error ? `${id}-error` : undefined}>
  <legend class="font-semibold text-text">{copy.text.overrides}</legend>
  <p class="text-sm text-text-muted">{copy.text.overrideEditorDescription}</p>
  {#each values as override, index (rowKeys.at(index))}
    <section class="grid min-w-0 gap-3 rounded-lg border border-border p-3" role="group" aria-label={copy.format.override(index)}>
      <div class="flex min-w-0 flex-wrap items-end gap-2">
        <div class="min-w-0 flex-1">
          <TextField id={`${id}-${rowKeys.at(index)}-name`} label={copy.format.overrideName(index)} value={override.name} oninput={(event) => update(index, {...cloneOverride(override), name: event.currentTarget.value})} />
        </div>
        <Button label={copy.format.removeOverride(index)} tone="secondary" onclick={() => remove(index)} />
      </div>
      <Collapsible triggerLabel={copy.text.processingParameters} open>
        <ProcessingChannelFields id={`${id}-${rowKeys.at(index)}-channel`} value={override.channel} onChange={(channel) => update(index, {...cloneOverride(override), channel})} />
      </Collapsible>
    </section>
  {/each}
  <Button label={copy.text.addOverride} tone="secondary" class="justify-self-start" onclick={() => onChange([...values.map(cloneOverride), {name: '', channel: structuredClone(emptyChannel)}])} />
  {#if error}<p class="text-sm text-danger" id={`${id}-error`} role="alert">{error}</p>{/if}
</fieldset>
