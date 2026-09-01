import type {ModuleStatus} from '../shared/module-state.js'
import type {ProblemView} from '../../presentation/problem.js'
import type {SettingsCandidate} from '../../wails/types.js'
import type {SettingsField} from './fields.js'

export interface SettingsModuleState {
  readonly status: ModuleStatus
  readonly server: DeepReadonly<SettingsCandidate> | null
  readonly draft: DeepReadonly<SettingsCandidate> | null
  readonly revision: number | null
  readonly fileRevision: number | null
  readonly updatedAt: string | null
  readonly problem: ProblemView | null
  readonly fieldProblems: ReadonlyMap<SettingsField, ProblemView>
  readonly dirty: boolean
  readonly validating: boolean
  readonly saving: boolean
  readonly restartRequired: boolean
  readonly conflict: boolean
}

export type DeepReadonly<T> = T extends (...args: never[]) => unknown
  ? T
  : T extends readonly (infer Item)[]
    ? readonly DeepReadonly<Item>[]
    : T extends object
      ? {readonly [Key in keyof T]: DeepReadonly<T[Key]>}
      : T

export interface SettingsModule {
  readonly state: SettingsModuleState
  start(): Promise<void>
  refresh(): Promise<void>
  dispose(): void
  updateDraft(update: (draft: SettingsCandidate) => void): void
  validate(field?: SettingsField): Promise<boolean>
  save(): Promise<boolean>
  reload(): Promise<boolean>
}
