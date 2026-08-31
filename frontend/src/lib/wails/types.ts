export interface ProblemWire {
  code: string
  message: string
  field?: string
  currentRevision?: number
}

export interface RuntimeWire {
  revision: number
  updatedAt: string
  phase: string
  platformSupported: boolean
  application?: RuntimeApplicationWire | null
  problem?: ProblemWire | null
}

export interface RuntimeApplicationWire {
  lifecycle: string
  avatarId: string
  planGeneration: number
  planStatus: string
  planSource: string
  configPath: string
  configId: string
  generationExhausted: boolean
  osc: RuntimeOscWire
  pluginFailures: PluginControlFailureWire[]
  planError?: string
  runtimeError?: string
}

export interface RuntimeOscWire {
  running: boolean
  connected: boolean
  hasTarget: boolean
  targetMode: string
  target: OscTargetWire
  lastError?: string
}

export interface OscTargetWire {
  host: string
  port: number
}

export interface PluginControlFailureWire {
  pluginId: string
  operation: string
  message: string
}

export interface PluginListWire {
  revision: number
  updatedAt: string
  plugins: PluginWire[]
  problem?: ProblemWire | null
}

export interface PluginWire {
  id: string
  name: string
  description: string
  version: string
  capabilities: string[]
  enabled: boolean
  active: boolean
  state: string
  configRevision: number
  frameRate: number
  consecutiveFailures: number
  restartCount: number
  startedAt?: string | null
  lastHeartbeatAt?: string | null
  lastFrameAt?: string | null
  nextRestartAt?: string | null
  lastError?: string
}

export interface PluginConfigWire {
  revision: number
  updatedAt: string
  pluginId: string
  configRevision: number
  data: string
  problem?: ProblemWire | null
}

export interface PluginMutationWire {
  revision: number
  updatedAt: string
  pluginId: string
  problem?: ProblemWire | null
}

export interface SettingsWire {
  revision: number
  updatedAt: string
  fileRevision: number
  settings: SettingsCandidate
  problem?: ProblemWire | null
}

export interface SettingsValidationWire {
  revision: number
  updatedAt: string
  settings: SettingsCandidate
  problem?: ProblemWire | null
}

export interface SettingsSaveWire {
  revision: number
  updatedAt: string
  fileRevision: number
  settings: SettingsCandidate
  restartRequired: boolean
  problem?: ProblemWire | null
}

export interface SettingsCandidate {
  avatar: AvatarSettingsWire
  plugins: PluginsSettingsWire
  processing: ProcessingSettingsWire
  osc: OscSettingsWire
}

export interface AvatarSettingsWire {
  oscRoot: string
  fallbackPath: string
}

export interface PluginsSettingsWire {
  devRoots: string[]
}

export interface ProcessingSettingsWire {
  defaultChannel: ProcessingChannelWire
  overrides: ProcessingOverrideWire[]
  activeStaleAfterMs: number
  mutualExclusion: string[][]
}

export interface ProcessingOverrideWire {
  name: string
  channel: ProcessingChannelWire
}

export interface ProcessingChannelWire {
  calibration: CalibrationWire
  tuning: TuningWire
  filter: FilterWire
  dropout: DropoutWire
}

export interface CalibrationWire {
  enabled: boolean
  neutral: number
  min: number
  max: number
  gain: number
  invert: boolean
}

export interface TuningWire {
  deadzone: number
  gain: number
  exponent: number
  clampEnabled: boolean
  clampMin: number
  clampMax: number
}

export interface FilterWire {
  mode: string
  emaAlpha: number
  minCutoff: number
  beta: number
  derivativeCutoff: number
}

export interface DropoutWire {
  holdDurationMs: number
  decayDurationMs: number
  staleAfterMs: number
}

export interface OscSettingsWire {
  targetMode: string
  preferredService: string
  manualHost: string
  manualPort: number
}
