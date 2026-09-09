/** Frontend-owned editable values. Go nil slices are normalized at the module boundary. */
export interface SettingsCandidate {
  avatar: {oscRoot: string; fallbackPath: string}
  plugins: {devRoots: string[]}
  processing: {
    defaultChannel: ProcessingChannel
    overrides: ProcessingOverride[]
    activeStaleAfterMs: number
    mutualExclusion: string[][]
  }
  osc: {targetMode: string; preferredService: string; manualHost: string; manualPort: number}
}

export interface ProcessingOverride {name: string; channel: ProcessingChannel}
export interface ProcessingChannel {
  calibration: {enabled: boolean; neutral: number; min: number; max: number; gain: number; invert: boolean}
  tuning: {deadzone: number; gain: number; exponent: number; clampEnabled: boolean; clampMin: number; clampMax: number}
  filter: {mode: string; emaAlpha: number; minCutoff: number; beta: number; derivativeCutoff: number}
  dropout: {holdDurationMs: number; decayDurationMs: number; staleAfterMs: number}
}
