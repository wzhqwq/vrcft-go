<script lang="ts">
  import {Tabs as BitsTabs} from 'bits-ui'
  import {copy} from '../../../copy/zh-CN.js'
  import {ResponsiveGrid} from '../../components/layout/index.js'
  import {NumberField, SelectField, SwitchField} from '../../components/ui/index.js'
  import type {ProcessingChannel} from '../../modules/settings/form.js'

  type ProcessingStage = 'calibration' | 'tuning' | 'filter' | 'dropout'
  type Props = {
    value: Readonly<ProcessingChannel>;
    id?: string;
    onChange: (value: ProcessingChannel) => void;
  }

  const stages = [
    {value: 'calibration', label: copy.text.calibration},
    {value: 'tuning', label: copy.text.tuning},
    {value: 'filter', label: copy.text.filter},
    {value: 'dropout', label: copy.text.dropout},
  ] satisfies Array<{value: ProcessingStage; label: string}>
  const filterModes = [
    {value: 'none', label: copy.text.noFilter},
    {value: 'ema', label: 'EMA'},
    {value: 'one_euro', label: 'One Euro'},
  ]

  let {value, id = 'processing-channel', onChange}: Props = $props()
  let stage = $state<ProcessingStage>('calibration')

  function update(next: ProcessingChannel) {
    onChange({
      calibration: {...next.calibration},
      tuning: {...next.tuning},
      filter: {...next.filter},
      dropout: {...next.dropout},
    })
  }

  function calibration(key: keyof ProcessingChannel['calibration'], next: number | boolean) {
    update({...value, calibration: {...value.calibration, [key]: next}})
  }

  function tuning(key: keyof ProcessingChannel['tuning'], next: number | boolean) {
    update({...value, tuning: {...value.tuning, [key]: next}})
  }

  function filter(key: keyof ProcessingChannel['filter'], next: number | string) {
    update({...value, filter: {...value.filter, [key]: next}})
  }

  function dropout(key: keyof ProcessingChannel['dropout'], next: number) {
    update({...value, dropout: {...value.dropout, [key]: next}})
  }
</script>

<div class="min-w-0" id={id} tabindex="-1">
  <BitsTabs.Root bind:value={stage} orientation="vertical" class="grid min-w-0 grid-cols-[9rem_minmax(0,1fr)] gap-4">
    <BitsTabs.List class="flex min-w-0 flex-col gap-1 rounded-lg border border-border bg-surface p-1">
      {#each stages as item (item.value)}
        <BitsTabs.Trigger
          value={item.value}
          class="focus-ring min-w-0 rounded-md px-3 py-2 text-left font-semibold text-text-muted transition-colors data-[state=active]:bg-surface-raised data-[state=active]:text-text"
        >
          {item.label}
        </BitsTabs.Trigger>
      {/each}
    </BitsTabs.List>
    <BitsTabs.Content value={stage} class="min-w-0">
      {#if stage === 'calibration'}
        <ResponsiveGrid>
          <div class="col-span-full"><SwitchField label={copy.text.enableCalibration} description={copy.text.enableCalibrationDescription} bind:checked={() => value.calibration.enabled, (next) => calibration('enabled', next)} /></div>
          {#if value.calibration.enabled}
            <NumberField label={copy.text.neutral} description={copy.text.neutralDescription} value={value.calibration.neutral} step="any" oninput={(event) => calibration('neutral', event.currentTarget.valueAsNumber)} />
            <NumberField label={copy.text.min} description={copy.text.minDescription} value={value.calibration.min} step="any" oninput={(event) => calibration('min', event.currentTarget.valueAsNumber)} />
            <NumberField label={copy.text.max} description={copy.text.maxDescription} value={value.calibration.max} step="any" oninput={(event) => calibration('max', event.currentTarget.valueAsNumber)} />
            <NumberField label={copy.text.calibrationGain} description={copy.text.calibrationGainDescription} value={value.calibration.gain} step="any" oninput={(event) => calibration('gain', event.currentTarget.valueAsNumber)} />
            <div class="col-span-full"><SwitchField label={copy.text.invertCalibration} description={copy.text.invertCalibrationDescription} bind:checked={() => value.calibration.invert, (next) => calibration('invert', next)} /></div>
          {/if}
        </ResponsiveGrid>
      {:else if stage === 'tuning'}
        <ResponsiveGrid>
          <NumberField label={copy.text.deadzone} description={copy.text.deadzoneDescription} value={value.tuning.deadzone} step="any" oninput={(event) => tuning('deadzone', event.currentTarget.valueAsNumber)} />
          <NumberField label={copy.text.tuningGain} description={copy.text.tuningGainDescription} value={value.tuning.gain} step="any" oninput={(event) => tuning('gain', event.currentTarget.valueAsNumber)} />
          <NumberField label={copy.text.exponent} description={copy.text.exponentDescription} value={value.tuning.exponent} step="any" oninput={(event) => tuning('exponent', event.currentTarget.valueAsNumber)} />
          <div class="col-span-full"><SwitchField label={copy.text.enableClamp} description={copy.text.enableClampDescription} bind:checked={() => value.tuning.clampEnabled, (next) => tuning('clampEnabled', next)} /></div>
          {#if value.tuning.clampEnabled}
            <NumberField label={copy.text.clampMin} value={value.tuning.clampMin} step="any" oninput={(event) => tuning('clampMin', event.currentTarget.valueAsNumber)} />
            <NumberField label={copy.text.clampMax} value={value.tuning.clampMax} step="any" oninput={(event) => tuning('clampMax', event.currentTarget.valueAsNumber)} />
          {/if}
        </ResponsiveGrid>
      {:else if stage === 'filter'}
        <ResponsiveGrid>
          <SelectField label={copy.text.filterMode} description={copy.text.filterModeDescription} value={value.filter.mode} options={filterModes} onValueChange={(next) => filter('mode', next)} />
          {#if value.filter.mode === 'ema'}
            <NumberField label={copy.text.emaAlpha} description={copy.text.emaAlphaDescription} value={value.filter.emaAlpha} step="any" oninput={(event) => filter('emaAlpha', event.currentTarget.valueAsNumber)} />
          {:else if value.filter.mode === 'one_euro'}
            <NumberField label={copy.text.minCutoff} description={copy.text.minCutoffDescription} value={value.filter.minCutoff} step="any" oninput={(event) => filter('minCutoff', event.currentTarget.valueAsNumber)} />
            <NumberField label={copy.text.filterBeta} description={copy.text.filterBetaDescription} value={value.filter.beta} step="any" oninput={(event) => filter('beta', event.currentTarget.valueAsNumber)} />
            <NumberField label={copy.text.derivativeCutoff} description={copy.text.derivativeCutoffDescription} value={value.filter.derivativeCutoff} step="any" oninput={(event) => filter('derivativeCutoff', event.currentTarget.valueAsNumber)} />
          {/if}
        </ResponsiveGrid>
      {:else}
        <ResponsiveGrid>
          <NumberField label={copy.text.holdDuration} description={copy.text.holdDurationDescription} value={value.dropout.holdDurationMs} min={0} step={1} oninput={(event) => dropout('holdDurationMs', event.currentTarget.valueAsNumber)} />
          <NumberField label={copy.text.decayDuration} description={copy.text.decayDurationDescription} value={value.dropout.decayDurationMs} min={0} step={1} oninput={(event) => dropout('decayDurationMs', event.currentTarget.valueAsNumber)} />
          <NumberField label={copy.text.staleAfter} description={copy.text.staleAfterDescription} value={value.dropout.staleAfterMs} min={0} step={1} oninput={(event) => dropout('staleAfterMs', event.currentTarget.valueAsNumber)} />
        </ResponsiveGrid>
      {/if}
    </BitsTabs.Content>
  </BitsTabs.Root>
</div>
