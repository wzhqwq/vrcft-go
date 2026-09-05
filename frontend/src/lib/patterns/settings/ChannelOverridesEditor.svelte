<script lang="ts">
  import {Button, Collapsible, TextField} from '../../components/ui/index.js'
  import type {ProcessingChannelWire, ProcessingOverrideWire} from '../../wails/types.js'
  import ProcessingChannelFields from './ProcessingChannelFields.svelte'

  type Props = {
    values: readonly Readonly<ProcessingOverrideWire>[];
    id?: string;
    error?: string;
    onChange: (values: ProcessingOverrideWire[]) => void;
  }

  const emptyChannel: ProcessingChannelWire = {
    calibration: {enabled: false, neutral: 0, min: 0, max: 1, gain: 1, invert: false},
    tuning: {deadzone: 0, gain: 1, exponent: 1, clampEnabled: false, clampMin: 0, clampMax: 1},
    filter: {mode: 'ema', emaAlpha: 0.5, minCutoff: 1, beta: 0, derivativeCutoff: 1},
    dropout: {holdDurationMs: 0, decayDurationMs: 0, staleAfterMs: 0},
  }

  let {values, id = 'channel-overrides', error, onChange}: Props = $props()

  function cloneOverride(value: Readonly<ProcessingOverrideWire>): ProcessingOverrideWire {
    return {name: value.name, channel: {
      calibration: {...value.channel.calibration}, tuning: {...value.channel.tuning},
      filter: {...value.channel.filter}, dropout: {...value.channel.dropout},
    }}
  }

  function update(index: number, next: ProcessingOverrideWire) {
    onChange(values.map((value, current) => current === index ? cloneOverride(next) : cloneOverride(value)))
  }
</script>

<fieldset class="grid min-w-0 gap-3" id={id} tabindex="-1" aria-describedby={error ? `${id}-error` : undefined}>
  <legend class="font-semibold text-text">通道覆盖</legend>
  <p class="text-sm text-text-muted">仅为指定通道替换默认处理参数。</p>
  {#each values as override, index (`${index}-${override.name}`)}
    <section class="grid min-w-0 gap-3 rounded-lg border border-border p-3" role="group" aria-label={`通道覆盖 ${index}`}>
      <div class="flex min-w-0 flex-wrap items-end gap-2">
        <div class="min-w-0 flex-1">
          <TextField id={`${id}-${index}-name`} label={`覆盖通道名称 ${index}`} value={override.name} oninput={(event) => update(index, {...cloneOverride(override), name: event.currentTarget.value})} />
        </div>
        <Button label={`删除通道覆盖 ${index}`} tone="secondary" onclick={() => onChange(values.filter((_, current) => current !== index).map(cloneOverride))} />
      </div>
      <Collapsible triggerLabel="处理参数" open>
        <ProcessingChannelFields id={`${id}-${index}-channel`} value={override.channel} onChange={(channel) => update(index, {...cloneOverride(override), channel})} />
      </Collapsible>
    </section>
  {/each}
  <Button label="添加通道覆盖" tone="secondary" class="justify-self-start" onclick={() => onChange([...values.map(cloneOverride), {name: '', channel: structuredClone(emptyChannel)}])} />
  {#if error}<p class="text-sm text-danger" id={`${id}-error`} role="alert">{error}</p>{/if}
</fieldset>
