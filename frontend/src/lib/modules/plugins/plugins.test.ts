import {describe, expect, it, vi} from 'vitest'

import {selectPlugins} from './selectors.js'
import {createPluginsModule} from './index.js'
import type {PluginsPort, Stop} from '../../wails/ports.js'
import type {PluginListWire, PluginMutationWire, PluginWire, ProblemWire} from '../../wails/types.js'

class PluginsMock implements PluginsPort {
  readonly listeners = new Set<(value: unknown) => void>()
  readonly listPending: Array<Deferred<PluginListWire>> = []
  readonly mutationPending: Array<Deferred<PluginMutationWire>> = []
  readonly setEnabledCalls: Array<{pluginId: string; enabled: boolean}> = []
  listCalls = 0
  subscriptionCalls = 0
  stopCalls = 0

  list(): Promise<PluginListWire> {
    this.listCalls += 1
    return deferredInto(this.listPending)
  }

  setEnabled(pluginId: string, enabled: boolean): Promise<PluginMutationWire> {
    this.setEnabledCalls.push({pluginId, enabled})
    return deferredInto(this.mutationPending)
  }

  onChanged(listener: (value: unknown) => void): Stop {
    this.subscriptionCalls += 1
    this.listeners.add(listener)
    let stopped = false

    return () => {
      if (stopped) return
      stopped = true
      this.stopCalls += 1
      this.listeners.delete(listener)
    }
  }

  emit(value: unknown) {
    for (const listener of this.listeners) listener(value)
  }
}

interface Deferred<T> {
  resolve(value: T): void
  reject(reason: unknown): void
}

function deferredInto<T>(pending: Array<Deferred<T>>): Promise<T> {
  return new Promise((resolve, reject) => pending.push({resolve, reject}))
}

function plugin(id: string, overrides: Partial<PluginWire> = {}): PluginWire {
  return {
    id,
    name: id,
    description: `${id} description`,
    version: '1.0.0',
    capabilities: ['Eye'],
    enabled: true,
    active: true,
    state: 'running',
    configRevision: 1,
    frameRate: 60,
    consecutiveFailures: 0,
    restartCount: 0,
    ...overrides,
  }
}

function listWire(revision: number, plugins: PluginWire[], problem?: ProblemWire): PluginListWire {
  return {
    revision,
    updatedAt: `2026-08-31T00:00:${String(revision).padStart(2, '0')}Z`,
    plugins,
    problem,
  }
}

function mutationWire(revision: number, pluginId: string, problem?: ProblemWire): PluginMutationWire {
  return {
    revision,
    updatedAt: `2026-08-31T00:01:${String(revision).padStart(2, '0')}Z`,
    pluginId,
    problem,
  }
}

async function startWith(mock: PluginsMock, wire: PluginListWire) {
  const module = createPluginsModule(mock)
  const started = module.start()
  mock.listPending[0]?.resolve(wire)
  await started
  return module
}

describe('plugin selection', () => {
  const fixtures = [
    plugin('healthy', {name: 'Zulu'}),
    plugin('backoff', {name: 'Alpha', state: 'backoff', consecutiveFailures: 1}),
    plugin('crashed', {name: 'Beta', state: 'crashed', lastError: 'process exited'}),
  ]

  it('sorts problem and recovery states first, then stably by name and ID', () => {
    const duplicateName = plugin('healthy-2', {name: 'Zulu'})
    const result = selectPlugins([...fixtures, duplicateName], {
      query: '', filter: 'all', page: 1, pageSize: 24,
    })

    expect(result.items.map((item) => item.id)).toEqual(['crashed', 'backoff', 'healthy', 'healthy-2'])
  })

  it('searches names and IDs case-insensitively and applies every filter', () => {
    expect(selectPlugins(fixtures, {query: 'ALP', filter: 'all', page: 1, pageSize: 24}).items.map((x) => x.id))
      .toEqual(['backoff'])
    expect(selectPlugins(fixtures, {query: 'CRASH', filter: 'all', page: 1, pageSize: 24}).items.map((x) => x.id))
      .toEqual(['crashed'])
    expect(selectPlugins([...fixtures, plugin('off', {enabled: false})], {query: '', filter: 'disabled', page: 1, pageSize: 24}).items.map((x) => x.id))
      .toEqual(['off'])
    expect(selectPlugins([...fixtures, plugin('off', {enabled: false})], {query: '', filter: 'enabled', page: 1, pageSize: 24}).items.map((x) => x.id))
      .toEqual(['crashed', 'backoff', 'healthy'])
    expect(selectPlugins(fixtures, {query: '', filter: 'problem', page: 1, pageSize: 24}).items.map((x) => x.id))
      .toEqual(['crashed', 'backoff'])
  })

  it('uses stable 24-item pagination and clamps invalid pages', () => {
    const fixtures = Array.from({length: 50}, (_, index) => plugin(`plugin-${String(index).padStart(2, '0')}`))

    expect(selectPlugins(fixtures, {query: '', filter: 'all', page: 2, pageSize: 24})).toMatchObject({
      page: 2, pageCount: 3, total: 50,
    })
    expect(selectPlugins(fixtures, {query: '', filter: 'all', page: 2, pageSize: 24}).items).toHaveLength(24)
    expect(selectPlugins(fixtures, {query: '', filter: 'all', page: 99, pageSize: 24})).toMatchObject({page: 3})
    expect(selectPlugins([], {query: '', filter: 'all', page: -4, pageSize: 24})).toMatchObject({page: 1, pageCount: 1})
  })
})

describe('Plugins module', () => {
  it('treats events as invalidation hints and owns exactly one idempotent subscription', async () => {
    const mock = new PluginsMock()
    const module = await startWith(mock, listWire(1, [plugin('initial')]))

    await module.start()
    mock.emit(listWire(99, [plugin('event-payload')]))

    expect(mock.subscriptionCalls).toBe(1)
    expect(mock.listCalls).toBe(2)
    mock.listPending[1]?.resolve(listWire(2, [plugin('queried')]))
    await vi.waitFor(() => expect(module.state.snapshot?.plugins[0]?.id).toBe('queried'))

    module.dispose()
    module.dispose()
    mock.emit({revision: 3})
    expect(mock.stopCalls).toBe(1)
    expect(mock.listCalls).toBe(2)
  })

  it('lets only the latest-issued list request affect state, including failures and invalid revisions', async () => {
    const mock = new PluginsMock()
    const module = await startWith(mock, listWire(3, [plugin('initial')]))
    const old = module.refresh()
    const latest = module.refresh()

    mock.listPending[2]?.resolve(listWire(4, [plugin('latest')]))
    await latest
    mock.listPending[1]?.resolve(listWire(5, [plugin('late-old')]))
    await old
    expect(module.state.snapshot?.plugins[0]?.id).toBe('latest')

    const superseded = module.refresh()
    const current = module.refresh()
    mock.listPending[3]?.reject(new Error('superseded failure'))
    await superseded
    expect(module.state.status).toBe('ready')
    mock.listPending[4]?.resolve(listWire(Number.MAX_SAFE_INTEGER + 1, [plugin('unsafe')]))
    await current
    expect(module.state).toMatchObject({status: 'stale', revision: 4})
    expect(module.state.snapshot?.plugins[0]?.id).toBe('latest')
  })

  it('retains a valid list on refresh failure and reports initial failure without data', async () => {
    const firstMock = new PluginsMock()
    const initialFailure = createPluginsModule(firstMock)
    const starting = initialFailure.start()
    firstMock.listPending[0]?.reject(new Error('offline'))
    await starting
    expect(initialFailure.state).toMatchObject({status: 'problem', snapshot: null, revision: null})

    const mock = new PluginsMock()
    const module = await startWith(mock, listWire(7, [plugin('kept')]))
    const refreshing = module.refresh()
    mock.listPending[1]?.reject(new Error('offline'))
    await refreshing
    expect(module.state).toMatchObject({status: 'stale', revision: 7})
    expect(module.state.snapshot?.plugins[0]?.id).toBe('kept')
  })

  it('resets page one on query/filter changes and exposes a derived summary', async () => {
    const mock = new PluginsMock()
    const entries = [
      plugin('active'),
      plugin('inactive', {active: false}),
      plugin('off', {enabled: false, active: false}),
      plugin('bad', {active: false, state: 'crashed', lastError: 'boom'}),
    ]
    const module = await startWith(mock, listWire(1, entries))

    module.setPage(3)
    module.setQuery('active')
    expect(module.state.query.page).toBe(1)
    module.setPage(2)
    module.setFilter('problem')
    expect(module.state.query.page).toBe(1)
    expect(module.state.summary).toEqual({total: 4, enabled: 3, active: 1, problem: 1})
    module.setQuery('')
    expect(module.state.visiblePlugins.map((item) => item.id)).toEqual(['bad'])
  })

  it('isolates pending and failures per ID while allowing different IDs concurrently', async () => {
    const mock = new PluginsMock()
    const module = await startWith(mock, listWire(1, [plugin('eye'), plugin('lip')]))
    const eye = module.setEnabled('eye', false)
    const lip = module.setEnabled('lip', false)

    expect([...module.state.pendingIds]).toEqual(['eye', 'lip'])
    mock.mutationPending[0]?.resolve(mutationWire(2, 'eye', {code: 'unavailable', message: 'eye busy'}))
    await eye
    expect(module.state.pendingIds.has('eye')).toBe(false)
    expect(module.state.pendingIds.has('lip')).toBe(true)
    expect(module.state.problems.get('eye')).toMatchObject({code: 'unavailable', detail: 'eye busy'})
    expect(module.state.snapshot?.plugins.map((item) => item.id)).toEqual(['eye', 'lip'])

    mock.mutationPending[1]?.reject(new Error('transport'))
    await lip
    expect(module.state.problems.get('lip')).toMatchObject({code: 'internal'})
  })

  it('coalesces same-ID commands until the first operation settles', async () => {
    const mock = new PluginsMock()
    const module = await startWith(mock, listWire(1, [plugin('eye')]))

    const first = module.setEnabled('eye', false)
    const second = module.setEnabled('eye', true)
    expect(second).toBe(first)
    expect(mock.setEnabledCalls).toEqual([{pluginId: 'eye', enabled: false}])

    mock.mutationPending[0]?.resolve(mutationWire(2, 'eye', {code: 'conflict', message: 'busy'}))
    await first
  })

  it('accepts mutation revision and performs exactly one list refresh on success', async () => {
    const mock = new PluginsMock()
    const module = await startWith(mock, listWire(1, [plugin('eye')]))
    const command = module.setEnabled('eye', false)

    mock.mutationPending[0]?.resolve(mutationWire(2, 'eye'))
    await vi.waitFor(() => expect(mock.listCalls).toBe(2))
    expect(module.state.revision).toBe(2)
    mock.listPending[1]?.resolve(listWire(3, [plugin('eye', {enabled: false, active: false})]))
    await command

    expect(mock.listCalls).toBe(2)
    expect(module.state.revision).toBe(3)
    expect(module.state.snapshot?.plugins[0]?.enabled).toBe(false)
    expect(module.state.pendingIds.has('eye')).toBe(false)
  })

  it('rejects mismatched or unsafe mutation responses without refreshing', async () => {
    const mock = new PluginsMock()
    const module = await startWith(mock, listWire(3, [plugin('eye')]))
    const command = module.setEnabled('eye', false)
    mock.mutationPending[0]?.resolve(mutationWire(Number.NaN, 'other'))
    await command

    expect(mock.listCalls).toBe(1)
    expect(module.state.problems.get('eye')).toMatchObject({code: 'internal'})
    expect(module.state.revision).toBe(3)
  })

  it('does not expose mutable internal arrays, sets, or maps', async () => {
    const mock = new PluginsMock()
    const source = plugin('eye', {capabilities: ['Eye', 'Lip']})
    const module = await startWith(mock, listWire(1, [source]))

    source.name = 'mutated upstream'
    source.capabilities.push('Expression')
    const exposedPlugins = module.state.snapshot?.plugins as Array<PluginWire>
    expect(() => exposedPlugins.push(plugin('injected'))).toThrow()
    const pendingCopy = module.state.pendingIds as Set<string>
    pendingCopy.add('injected')
    const problemCopy = module.state.problems as Map<string, unknown>
    problemCopy.set('injected', {})

    expect(module.state.snapshot?.plugins[0]).toMatchObject({name: 'eye', capabilities: ['Eye', 'Lip']})
    expect(module.state.pendingIds.has('injected')).toBe(false)
    expect(module.state.problems.has('injected')).toBe(false)
  })
})
