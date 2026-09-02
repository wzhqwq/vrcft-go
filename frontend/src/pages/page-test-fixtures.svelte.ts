import type {PluginsModule, PluginsModuleState} from '../lib/modules/plugins/types.js'
import type {RuntimeModule, RuntimeModuleState} from '../lib/modules/runtime/types.js'

export interface PluginsCommands {
  setQuery(query: string): void
  setFilter(filter: PluginsModuleState['query']['filter']): void
  setPage(page: number): void
  setEnabled(pluginId: string, enabled: boolean): Promise<void>
}

export function createRuntimeFixture(initial: RuntimeModuleState) {
  let state = $state(initial)
  const module: RuntimeModule = {
    get state() { return state },
    start: async () => {},
    refresh: async () => {},
    dispose: () => {},
  }

  return {module, setState: (next: RuntimeModuleState) => { state = next }}
}

export function createPluginsFixture(initial: PluginsModuleState, commands: PluginsCommands) {
  let state = $state(initial)
  const module: PluginsModule = {
    get state() { return state },
    start: async () => {},
    refresh: async () => {},
    dispose: () => {},
    ...commands,
  }

  return {module, setState: (next: PluginsModuleState) => { state = next }}
}
