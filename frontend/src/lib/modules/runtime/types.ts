import type {ModuleState} from '../shared/module-state.js'

export type OscState = 'not_running' | 'discovering' | 'discovered' | 'manual' | 'error'

export interface RuntimeAvatarView {
  readonly id: string
  readonly name: string
}

export interface RuntimeOscTargetView {
  readonly host: string
  readonly port: number
}

export interface RuntimeOscView {
  readonly state: OscState
  readonly target?: RuntimeOscTargetView
  readonly error?: string
}

export interface RuntimePlanView {
  readonly generation: number
  readonly status: string
  readonly source: string
  readonly configPath: string
  readonly configId: string
  readonly generationExhausted: boolean
}

export interface RuntimePluginFailureView {
  readonly pluginId: string
  readonly operation: string
  readonly message: string
}

export interface RuntimeView {
  readonly phase: string
  readonly platformSupported: boolean
  readonly avatar: RuntimeAvatarView
  readonly lifecycle?: string
  readonly plan?: RuntimePlanView
  readonly osc?: RuntimeOscView
  readonly pluginFailures: readonly RuntimePluginFailureView[]
  readonly planError?: string
  readonly runtimeError?: string
}

export type RuntimeModuleState = ModuleState<RuntimeView>

export interface RuntimeModule {
  readonly state: RuntimeModuleState
  start(): Promise<void>
  refresh(): Promise<void>
  dispose(): void
}
