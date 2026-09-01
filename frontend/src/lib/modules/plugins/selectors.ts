import type {PluginFilter, PluginQuery, PluginSelection, PluginSummary, PluginView} from './types.js'

const failedStates = new Set(['crashed', 'error', 'failed', 'unresponsive', 'incompatible'])
const recoveryStates = new Set(['backoff', 'restarting'])

export function pluginHasProblem(plugin: PluginView): boolean {
  return pluginHealthRank(plugin) < 2
}

export function selectPlugins(plugins: readonly PluginView[], query: PluginQuery): PluginSelection {
  const needle = query.query.trim().toLowerCase()
  const filtered = plugins.filter((plugin) => {
    if (!matchesFilter(plugin, query.filter)) return false
    if (needle === '') return true
    return plugin.name.toLowerCase().includes(needle) || plugin.id.toLowerCase().includes(needle)
  })
  const sorted = [...filtered].sort(comparePlugins)
  const pageCount = Math.max(1, Math.ceil(sorted.length / 24))
  const requestedPage = Number.isSafeInteger(query.page) ? query.page : 1
  const page = Math.min(Math.max(requestedPage, 1), pageCount)
  const offset = (page - 1) * 24

  return Object.freeze({
    items: Object.freeze(sorted.slice(offset, offset + 24)),
    page,
    pageCount,
    total: sorted.length,
  })
}

export function summarizePlugins(plugins: readonly PluginView[]): PluginSummary {
  return Object.freeze({
    total: plugins.length,
    enabled: plugins.filter((plugin) => plugin.enabled).length,
    active: plugins.filter((plugin) => plugin.active).length,
    problem: plugins.filter(pluginHasProblem).length,
  })
}

function matchesFilter(plugin: PluginView, filter: PluginFilter): boolean {
  switch (filter) {
    case 'enabled': return plugin.enabled
    case 'disabled': return !plugin.enabled
    case 'problem': return pluginHasProblem(plugin)
    default: return true
  }
}

function comparePlugins(left: PluginView, right: PluginView): number {
  const healthDifference = pluginHealthRank(left) - pluginHealthRank(right)
  if (healthDifference !== 0) return healthDifference

  const nameDifference = compareText(left.name, right.name)
  return nameDifference !== 0 ? nameDifference : compareText(left.id, right.id)
}

function pluginHealthRank(plugin: PluginView): number {
  const state = plugin.state.toLowerCase()
  if (failedStates.has(state)) return 0
  if (recoveryStates.has(state) || plugin.consecutiveFailures > 0 || Boolean(plugin.lastError)) return 1
  return 2
}

function compareText(left: string, right: string): number {
  const normalizedLeft = left.toLowerCase()
  const normalizedRight = right.toLowerCase()
  if (normalizedLeft < normalizedRight) return -1
  if (normalizedLeft > normalizedRight) return 1
  return left < right ? -1 : left > right ? 1 : 0
}
