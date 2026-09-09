<script lang="ts">
  import {copy} from '../../../copy/zh-CN.js'
  import {ResponsiveGrid} from '../../components/layout/index.js'
  import {Collapsible, NumberField, SelectField, SwitchField} from '../../components/ui/index.js'
  import type {ProcessingChannel} from '../../modules/settings/form.js'

  type Props = {
    value: Readonly<ProcessingChannel>;
    id?: string;
    onChange: (value: ProcessingChannel) => void;
  }

  const filterModes = [
    {value: 'none', label: copy.text.noFilter},
    {value: 'ema', label: 'EMA'},
    {value: 'one_euro', label: 'One Euro'},
  ]

  let {value, id = 'processing-channel', onChange}: Props = $props()

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

<div class="grid min-w-0 gap-3" id={id} tabindex="-1">
  <Collapsible triggerLabel={copy.text.calibration} open>
    <ResponsiveGrid class="pt-1">
      <div class="col-span-full"><SwitchField label={copy.text.enableCalibration} bind:checked={() => value.calibration.enabled, (next) => calibration('enabled', next)} /></div>
      <NumberField label={copy.text.neutral} value={value.calibration.neutral} step="any" oninput={(event) => calibration('neutral', event.currentTarget.valueAsNumber)} />
      <NumberField label={copy.text.min} value={value.calibration.min} step="any" oninput={(event) => calibration('min', event.currentTarget.valueAsNumber)} />
      <NumberField label={copy.text.max} value={value.calibration.max} step="any" oninput={(event) => calibration('max', event.currentTarget.valueAsNumber)} />
      <NumberField label={copy.text.calibrationGain} value={value.calibration.gain} step="any" oninput={(event) => calibration('gain', event.currentTarget.valueAsNumber)} />
      <div class="col-span-full"><SwitchField label={copy.text.invertCalibration} bind:checked={() => value.calibration.invert, (next) => calibration('invert', next)} /></div>
    </ResponsiveGrid>
  </Collapsible>

  <Collapsible triggerLabel={copy.text.tuning} open>
    <ResponsiveGrid class="pt-1">
      <NumberField label={copy.text.deadzone} value={value.tuning.deadzone} step="any" oninput={(event) => tuning('deadzone', event.currentTarget.valueAsNumber)} />
      <NumberField label={copy.text.tuningGain} value={value.tuning.gain} step="any" oninput={(event) => tuning('gain', event.currentTarget.valueAsNumber)} />
      <NumberField label={copy.text.exponent} value={value.tuning.exponent} step="any" oninput={(event) => tuning('exponent', event.currentTarget.valueAsNumber)} />
      <div class="col-span-full"><SwitchField label={copy.text.enableClamp} bind:checked={() => value.tuning.clampEnabled, (next) => tuning('clampEnabled', next)} /></div>
      <NumberField label={copy.text.clampMin} value={value.tuning.clampMin} step="any" oninput={(event) => tuning('clampMin', event.currentTarget.valueAsNumber)} />
      <NumberField label={copy.text.clampMax} value={value.tuning.clampMax} step="any" oninput={(event) => tuning('clampMax', event.currentTarget.valueAsNumber)} />
    </ResponsiveGrid>
  </Collapsible>

  <Collapsible triggerLabel={copy.text.filter} open>
    <ResponsiveGrid class="pt-1">
      <SelectField label={copy.text.filterMode} value={value.filter.mode} options={filterModes} onValueChange={(next) => filter('mode', next)} />
      <NumberField label={copy.text.emaAlpha} value={value.filter.emaAlpha} step="any" oninput={(event) => filter('emaAlpha', event.currentTarget.valueAsNumber)} />
      <NumberField label={copy.text.minCutoff} value={value.filter.minCutoff} step="any" oninput={(event) => filter('minCutoff', event.currentTarget.valueAsNumber)} />
      <NumberField label="Beta" value={value.filter.beta} step="any" oninput={(event) => filter('beta', event.currentTarget.valueAsNumber)} />
      <NumberField label={copy.text.derivativeCutoff} value={value.filter.derivativeCutoff} step="any" oninput={(event) => filter('derivativeCutoff', event.currentTarget.valueAsNumber)} />
    </ResponsiveGrid>
  </Collapsible>

  <Collapsible triggerLabel={copy.text.dropout} open>
    <ResponsiveGrid class="pt-1">
      <NumberField label={copy.text.holdDuration} value={value.dropout.holdDurationMs} min={0} step={1} oninput={(event) => dropout('holdDurationMs', event.currentTarget.valueAsNumber)} />
      <NumberField label={copy.text.decayDuration} value={value.dropout.decayDurationMs} min={0} step={1} oninput={(event) => dropout('decayDurationMs', event.currentTarget.valueAsNumber)} />
      <NumberField label={copy.text.staleAfter} value={value.dropout.staleAfterMs} min={0} step={1} oninput={(event) => dropout('staleAfterMs', event.currentTarget.valueAsNumber)} />
    </ResponsiveGrid>
  </Collapsible>
</div>
