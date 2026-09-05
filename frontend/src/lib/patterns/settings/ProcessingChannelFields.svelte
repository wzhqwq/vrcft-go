<script lang="ts">
  import {Collapsible, NumberField, SelectField, SwitchField} from '../../components/ui/index.js'
  import type {ProcessingChannelWire} from '../../wails/types.js'

  type Props = {
    value: Readonly<ProcessingChannelWire>;
    id?: string;
    onChange: (value: ProcessingChannelWire) => void;
  }

  const filterModes = [
    {value: 'none', label: '不滤波'},
    {value: 'ema', label: 'EMA'},
    {value: 'one_euro', label: 'One Euro'},
  ]

  let {value, id = 'processing-channel', onChange}: Props = $props()

  function update(next: ProcessingChannelWire) {
    onChange({
      calibration: {...next.calibration},
      tuning: {...next.tuning},
      filter: {...next.filter},
      dropout: {...next.dropout},
    })
  }

  function calibration(key: keyof ProcessingChannelWire['calibration'], next: number | boolean) {
    update({...value, calibration: {...value.calibration, [key]: next}})
  }

  function tuning(key: keyof ProcessingChannelWire['tuning'], next: number | boolean) {
    update({...value, tuning: {...value.tuning, [key]: next}})
  }

  function filter(key: keyof ProcessingChannelWire['filter'], next: number | string) {
    update({...value, filter: {...value.filter, [key]: next}})
  }

  function dropout(key: keyof ProcessingChannelWire['dropout'], next: number) {
    update({...value, dropout: {...value.dropout, [key]: next}})
  }
</script>

<div class="grid min-w-0 gap-3" id={id} tabindex="-1">
  <Collapsible triggerLabel="校准" open>
    <div class="grid min-w-0 gap-3 pt-1">
      <SwitchField label="启用校准" bind:checked={() => value.calibration.enabled, (next) => calibration('enabled', next)} />
      <NumberField label="中立值" value={value.calibration.neutral} step="any" oninput={(event) => calibration('neutral', event.currentTarget.valueAsNumber)} />
      <NumberField label="最小值" value={value.calibration.min} step="any" oninput={(event) => calibration('min', event.currentTarget.valueAsNumber)} />
      <NumberField label="最大值" value={value.calibration.max} step="any" oninput={(event) => calibration('max', event.currentTarget.valueAsNumber)} />
      <NumberField label="校准增益" value={value.calibration.gain} step="any" oninput={(event) => calibration('gain', event.currentTarget.valueAsNumber)} />
      <SwitchField label="反转校准" bind:checked={() => value.calibration.invert, (next) => calibration('invert', next)} />
    </div>
  </Collapsible>

  <Collapsible triggerLabel="调节" open>
    <div class="grid min-w-0 gap-3 pt-1">
      <NumberField label="死区" value={value.tuning.deadzone} step="any" oninput={(event) => tuning('deadzone', event.currentTarget.valueAsNumber)} />
      <NumberField label="调节增益" value={value.tuning.gain} step="any" oninput={(event) => tuning('gain', event.currentTarget.valueAsNumber)} />
      <NumberField label="指数" value={value.tuning.exponent} step="any" oninput={(event) => tuning('exponent', event.currentTarget.valueAsNumber)} />
      <SwitchField label="启用钳制" bind:checked={() => value.tuning.clampEnabled, (next) => tuning('clampEnabled', next)} />
      <NumberField label="钳制最小值" value={value.tuning.clampMin} step="any" oninput={(event) => tuning('clampMin', event.currentTarget.valueAsNumber)} />
      <NumberField label="钳制最大值" value={value.tuning.clampMax} step="any" oninput={(event) => tuning('clampMax', event.currentTarget.valueAsNumber)} />
    </div>
  </Collapsible>

  <Collapsible triggerLabel="滤波" open>
    <div class="grid min-w-0 gap-3 pt-1">
      <SelectField label="滤波模式" value={value.filter.mode} options={filterModes} onValueChange={(next) => filter('mode', next)} />
      <NumberField label="EMA 系数" value={value.filter.emaAlpha} step="any" oninput={(event) => filter('emaAlpha', event.currentTarget.valueAsNumber)} />
      <NumberField label="最小截止频率" value={value.filter.minCutoff} step="any" oninput={(event) => filter('minCutoff', event.currentTarget.valueAsNumber)} />
      <NumberField label="Beta" value={value.filter.beta} step="any" oninput={(event) => filter('beta', event.currentTarget.valueAsNumber)} />
      <NumberField label="导数截止频率" value={value.filter.derivativeCutoff} step="any" oninput={(event) => filter('derivativeCutoff', event.currentTarget.valueAsNumber)} />
    </div>
  </Collapsible>

  <Collapsible triggerLabel="丢失数据" open>
    <div class="grid min-w-0 gap-3 pt-1">
      <NumberField label="保持时长（毫秒）" value={value.dropout.holdDurationMs} min={0} step={1} oninput={(event) => dropout('holdDurationMs', event.currentTarget.valueAsNumber)} />
      <NumberField label="衰减时长（毫秒）" value={value.dropout.decayDurationMs} min={0} step={1} oninput={(event) => dropout('decayDurationMs', event.currentTarget.valueAsNumber)} />
      <NumberField label="过期时长（毫秒）" value={value.dropout.staleAfterMs} min={0} step={1} oninput={(event) => dropout('staleAfterMs', event.currentTarget.valueAsNumber)} />
    </div>
  </Collapsible>
</div>
