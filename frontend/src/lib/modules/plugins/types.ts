import type {ProblemView} from '../../presentation/problem.js'
import type {ModuleStatus} from '../shared/module-state.js'

export type PluginFilter = 'all' | 'enabled' | 'disabled' | 'problem'

export interface PluginQuery {
  readonly query: string
  readonly filter: PluginFilter
  readonly page: number
  readonly pageSize: 24
}

export interface PluginView {
  readonly id: string
  readonly name: string
  readonly description: string
  readonly version: string
  readonly capabilities: readonly string[]
  readonly enabled: boolean
  readonly active: boolean
  readonly state: string
  readonly configRevision: number
  readonly frameRate: number
  readonly consecutiveFailures: number
  readonly restartCount: number
  readonly startedAt?: string | null
  readonly lastHeartbeatAt?: string | null
  readonly lastFrameAt?: string | null
  readonly nextRestartAt?: string | null
  readonly lastError?: string
}

export interface PluginsView {
  readonly plugins: readonly PluginView[]
}

export interface PluginSummary {
  readonly total: number
  readonly enabled: number
  readonly active: number
  readonly problem: number
}

export interface PluginSelection {
  readonly items: readonly PluginView[]
  readonly page: number
  readonly pageCount: number
  readonly total: number
}

export interface PluginCommandState {
  readonly pending: ReadonlySet<string>
  readonly problems: ReadonlyMap<string, ProblemView>
}

export interface PluginsModuleState {
  readonly status: ModuleStatus
  readonly snapshot: PluginsView | null
  readonly revision: number | null
  readonly updatedAt: string | null
  readonly problem: ProblemView | null
  readonly query: PluginQuery
  readonly visiblePlugins: readonly PluginView[]
  readonly pageCount: number
  readonly filteredTotal: number
  readonly summary: PluginSummary
  readonly pendingIds: ReadonlySet<string>
  readonly problems: ReadonlyMap<string, ProblemView>
  readonly commands: PluginCommandState
}

export interface PluginsModule {
  readonly state: PluginsModuleState
  start(): Promise<void>
  refresh(): Promise<void>
  dispose(): void
  setQuery(query: string): void
  setFilter(filter: PluginFilter): void
  setPage(page: number): void
  setEnabled(pluginId: string, enabled: boolean): Promise<void>
}
