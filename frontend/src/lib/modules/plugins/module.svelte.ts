import {presentProblem, type ProblemView} from '../../presentation/problem.js'
import type {PluginsPort, Stop} from '../../wails/ports.js'
import type {PluginListWire, PluginWire} from '../../wails/types.js'

import {acceptRevision} from '../shared/revision.js'
import {selectPlugins, summarizePlugins} from './selectors.js'
import type {
  PluginFilter,
  PluginsModule,
  PluginsModuleState,
  PluginsView,
  PluginView,
} from './types.js'

interface MutablePluginsState {
  status: PluginsModuleState['status']
  snapshot: PluginsView | null
  revision: number | null
  updatedAt: string | null
  problem: ProblemView | null
  queryText: string
  filter: PluginFilter
  page: number
  pendingIds: string[]
  commandProblems: Array<readonly [string, ProblemView]>
}

export function createPluginsModule(port: PluginsPort): PluginsModule {
  const data = $state<MutablePluginsState>({
    status: 'loading',
    snapshot: null,
    revision: null,
    updatedAt: null,
    problem: null,
    queryText: '',
    filter: 'all',
    page: 1,
    pendingIds: [],
    commandProblems: [],
  })
  let started = false
  let disposed = false
  let startPromise: Promise<void> | null = null
  let stop: Stop | null = null
  let nextRequest = 0
  const operations = new Map<string, Promise<void>>()

  const selection = () => selectPlugins(data.snapshot?.plugins ?? [], {
    query: data.queryText,
    filter: data.filter,
    page: data.page,
    pageSize: 24,
  })
  const pendingCopy = () => new Set(data.pendingIds)
  const problemsCopy = () => new Map(data.commandProblems)

  const state: PluginsModuleState = {
    get status() { return data.status },
    get snapshot() { return data.snapshot },
    get revision() { return data.revision },
    get updatedAt() { return data.updatedAt },
    get problem() { return data.problem },
    get query() {
      return Object.freeze({query: data.queryText, filter: data.filter, page: selection().page, pageSize: 24 as const})
    },
    get visiblePlugins() { return selection().items },
    get pageCount() { return selection().pageCount },
    get filteredTotal() { return selection().total },
    get summary() { return summarizePlugins(data.snapshot?.plugins ?? []) },
    get pendingIds() { return pendingCopy() },
    get problems() { return problemsCopy() },
    get commands() {
      return Object.freeze({pending: pendingCopy(), problems: problemsCopy()})
    },
  }

  const refresh = async () => {
    if (disposed) return
    const request = ++nextRequest

    try {
      const wire = await port.list()
      if (disposed || request !== nextRequest) return
      if (!acceptRevision(data.revision ?? -1, wire.revision)) {
        markRefreshProblem()
        return
      }

      data.snapshot = mapPluginsWire(wire)
      data.revision = wire.revision
      data.updatedAt = wire.updatedAt
      data.problem = wire.problem == null ? null : freezeProblem(presentProblem(wire.problem))
      data.status = data.problem === null ? 'ready' : 'problem'
      data.page = selection().page
    } catch {
      if (disposed || request !== nextRequest) return
      markRefreshProblem()
    }
  }

  const setEnabled = (pluginId: string, enabled: boolean): Promise<void> => {
    const current = operations.get(pluginId)
    if (current !== undefined) return current

    data.pendingIds = [...data.pendingIds, pluginId]
    data.commandProblems = data.commandProblems.filter(([id]) => id !== pluginId)
    const operation = performSetEnabled(pluginId, enabled)
    operations.set(pluginId, operation)
    return operation
  }

  const module: PluginsModule = {
    state,
    start() {
      if (disposed) return Promise.resolve()
      if (started) return startPromise ?? Promise.resolve()
      started = true
      stop = port.onChanged(() => { void refresh() })
      startPromise = refresh()
      return startPromise
    },
    refresh,
    dispose() {
      if (disposed) return
      disposed = true
      const currentStop = stop
      stop = null
      currentStop?.()
    },
    setQuery(query) {
      data.queryText = query
      data.page = 1
    },
    setFilter(filter) {
      data.filter = filter
      data.page = 1
    },
    setPage(page) {
      data.page = selectPlugins(data.snapshot?.plugins ?? [], {
        query: data.queryText, filter: data.filter, page, pageSize: 24,
      }).page
    },
    setEnabled,
  }

  return module

  async function performSetEnabled(pluginId: string, enabled: boolean): Promise<void> {
    try {
      const result = await port.setEnabled(pluginId, enabled)
      if (disposed) return

      if (result.pluginId !== pluginId || !acceptRevision(data.revision ?? -1, result.revision)) {
        setCommandProblem(pluginId, internalProblem())
        return
      }
      if (result.problem != null) {
        setCommandProblem(pluginId, freezeProblem(presentProblem(result.problem)))
        return
      }

      data.revision = result.revision
      data.updatedAt = result.updatedAt
      await refresh()
    } catch {
      if (!disposed) setCommandProblem(pluginId, internalProblem())
    } finally {
      operations.delete(pluginId)
      if (!disposed) data.pendingIds = data.pendingIds.filter((id) => id !== pluginId)
    }
  }

  function markRefreshProblem() {
    data.problem = internalProblem()
    data.status = data.snapshot === null ? 'problem' : 'stale'
  }

  function setCommandProblem(pluginId: string, problem: ProblemView) {
    data.commandProblems = [...data.commandProblems.filter(([id]) => id !== pluginId), [pluginId, problem]]
  }
}

function mapPluginsWire(wire: PluginListWire): PluginsView {
  const plugins = Object.freeze(wire.plugins.map(mapPluginWire))
  return Object.freeze({plugins})
}

function mapPluginWire(plugin: PluginWire): PluginView {
  return Object.freeze({...plugin, capabilities: Object.freeze([...plugin.capabilities])})
}

function internalProblem(): ProblemView {
  return freezeProblem(presentProblem({code: 'internal', message: ''}))
}

function freezeProblem(problem: ProblemView): ProblemView {
  return Object.freeze(problem)
}
