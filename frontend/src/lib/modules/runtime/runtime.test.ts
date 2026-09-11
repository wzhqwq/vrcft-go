import {describe, expect, it, vi} from 'vitest'

import {acceptRevision} from '../shared/revision.js'
import type {RuntimePort, Stop} from '../../wails/ports.js'
import type {RuntimeApplicationWire, RuntimeOscWire, RuntimeWire} from '../../wails/types.js'
import {createRuntimeModule} from './index.js'

class RuntimeMock implements RuntimePort {
  readonly listeners = new Set<(value: unknown) => void>()
  readonly pending: Array<{
    resolve(value: RuntimeWire): void
    reject(reason: unknown): void
  }> = []
  getStatusCalls = 0
  subscriptionCalls = 0
  stopCalls = 0

  getStatus(): Promise<RuntimeWire> {
    this.getStatusCalls += 1
    return new Promise((resolve, reject) => {
      this.pending.push({resolve, reject})
    })
  }

  onChanged(listener: (value: unknown) => void): Stop {
    this.subscriptionCalls += 1
    this.listeners.add(listener)
    let stopped = false

    return () => {
      if (stopped) {
        return
      }

      stopped = true
      this.stopCalls += 1
      this.listeners.delete(listener)
    }
  }

  resolvePending(index: number, value: RuntimeWire) {
    this.pending[index]?.resolve(value)
  }

  rejectPending(index: number, reason: unknown) {
    this.pending[index]?.reject(reason)
  }

  emit(value: unknown) {
    for (const listener of this.listeners) {
      listener(value)
    }
  }
}

function runtimeWire(
  revision: number,
  avatarID = 'avtr_current',
  application: Partial<RuntimeApplicationWire> = {},
): RuntimeWire {
  return {
    revision,
    updatedAt: `2026-08-31T00:00:0${revision}Z`,
    phase: 'running',
    platformSupported: true,
    application: {
      lifecycle: 'running',
      avatarId: avatarID,
      avatarName: 'Demo Avatar',
      planGeneration: 4,
      planStatus: 'ready',
      planSource: 'local',
      configPath: 'C:/avatars/current.json',
      configId: 'cfg_current',
      generationExhausted: false,
      osc: {
        running: true,
        connected: true,
        hasTarget: true,
        targetMode: 'auto',
        target: {host: '127.0.0.1', port: 9000},
      },
      pluginFailures: [],
      ...application,
    },
  }
}

async function startWith(mock: RuntimeMock, wire: RuntimeWire) {
  const module = createRuntimeModule(mock)
  const started = module.start()
  mock.resolvePending(0, wire)
  await started
  return module
}

describe('Runtime module', () => {
  it('accepts legacy null plugin failure lists without losing the runtime revision', async () => {
    const module = await startWith(new RuntimeMock(), runtimeWire(1, 'avtr_current', {pluginFailures: null as unknown as []}))
    expect(module.state.status).toBe('ready')
    expect(module.state.revision).toBe(1)
    expect(module.state.snapshot?.pluginFailures).toEqual([])
  })

  it('reads diagnostics independently when status parsing fails and reports the parsing stage', async () => {
    const mock = new RuntimeMock()
    const reportFrontendError = vi.fn().mockResolvedValue(undefined)
    const module = createRuntimeModule(Object.assign(mock, {
      getDiagnostics: async () => ({entries: [], logPath: 'logs', diskError: ''}), reportFrontendError,
    }))
    const started = module.start()
    mock.resolvePending(0, runtimeWire(1, 'avtr_current', {osc: null as unknown as RuntimeOscWire}))
    await started
    await module.refreshDiagnostics?.()
    expect(module.state.problem?.detail).toContain('parse_status')
    expect(reportFrontendError).toHaveBeenCalledWith('parse_status', expect.any(String))
    expect(module.diagnostics?.snapshot?.logPath).toBe('logs')
  })
  it('distinguishes status requests and invalid revisions in diagnostic reports', async () => {
    const mock = new RuntimeMock()
    const reportFrontendError = vi.fn().mockResolvedValue(undefined)
    const module = createRuntimeModule(Object.assign(mock, {reportFrontendError}))
    const started = module.start()
    mock.rejectPending(0, new Error('transport failed token=private'))
    await started
    expect(module.state.problem?.detail).toContain('get_status')
    expect(reportFrontendError).toHaveBeenLastCalledWith('get_status', 'transport failed token=[REDACTED]')
    const refreshed = module.refresh()
    mock.resolvePending(1, runtimeWire(Number.NaN))
    await refreshed
    expect(module.state.problem?.detail).toContain('revision')
    expect(reportFrontendError).toHaveBeenLastCalledWith('revision', expect.any(String))
  })
  it('rejects unsafe and older revisions', () => {
    expect(acceptRevision(3, 3)).toBe(true)
    expect(acceptRevision(3, 4)).toBe(true)
    expect(acceptRevision(3, 2)).toBe(false)
    expect(acceptRevision(3, Number.MAX_SAFE_INTEGER + 1)).toBe(false)
    expect(acceptRevision(3, Number.NaN)).toBe(false)
  })

  it('maps the backend avatar display name without deriving one from its ID', async () => {
    const mock = new RuntimeMock()
    const module = await startWith(mock, runtimeWire(1, 'avtr_current', {avatarName: 'Demo Avatar'}))

    expect(module.state.snapshot?.avatar).toEqual({id: 'avtr_current', name: 'Demo Avatar'})
  })

  it('keeps the newer accepted snapshot when an older request resolves late', async () => {
    const mock = new RuntimeMock()
    const module = await startWith(mock, runtimeWire(1, 'avtr_initial'))
    const older = module.refresh()
    const newer = module.refresh()

    mock.resolvePending(2, runtimeWire(3, 'avtr_new'))
    await newer
    mock.resolvePending(1, runtimeWire(2, 'avtr_old'))
    await older

    expect(module.state.snapshot?.avatar.id).toBe('avtr_new')
    expect(module.state.revision).toBe(3)
  })

  it('does not mark a valid snapshot stale when a superseded refresh fails first', async () => {
    const mock = new RuntimeMock()
    const module = await startWith(mock, runtimeWire(1, 'avtr_initial'))
    const before = runtimeState(module)
    const older = module.refresh()
    const newer = module.refresh()

    mock.rejectPending(1, new Error('older request failed'))
    await older

    expect(runtimeState(module)).toEqual(before)

    mock.resolvePending(2, runtimeWire(2, 'avtr_newer'))
    await newer
  })

  it('does not overwrite a newer refresh failure when its superseded request succeeds', async () => {
    const mock = new RuntimeMock()
    const module = await startWith(mock, runtimeWire(1, 'avtr_initial'))
    const older = module.refresh()
    const newer = module.refresh()

    mock.rejectPending(2, new Error('newer request failed'))
    await newer
    const afterNewerFailure = runtimeState(module)

    mock.resolvePending(1, runtimeWire(2, 'avtr_superseded'))
    await older

    expect(runtimeState(module)).toEqual(afterNewerFailure)
  })

  it('treats runtime events as invalidation hints and makes one subscription', async () => {
    const mock = new RuntimeMock()
    const module = await startWith(mock, runtimeWire(1, 'avtr_initial'))

    await module.start()
    mock.emit(runtimeWire(99, 'avtr_event_payload'))

    expect(mock.subscriptionCalls).toBe(1)
    expect(mock.getStatusCalls).toBe(2)
    mock.resolvePending(1, runtimeWire(2, 'avtr_queried'))

    await vi.waitFor(() => expect(module.state.snapshot?.avatar.id).toBe('avtr_queried'))
    expect(module.state.revision).toBe(2)
  })

  it('reports an initial query failure as a no-data problem', async () => {
    const mock = new RuntimeMock()
    const module = createRuntimeModule(mock)
    const started = module.start()

    mock.rejectPending(0, new Error('transport unavailable'))
    await started

    expect(module.state).toMatchObject({
      status: 'problem',
      snapshot: null,
      revision: null,
      updatedAt: null,
      problem: {code: 'internal', tone: 'danger'},
    })
  })

  it('owns a synchronous subscription failure as an idempotent no-data problem', async () => {
    const mock = new RuntimeMock()
    vi.spyOn(mock, 'onChanged').mockImplementation(() => {
      mock.subscriptionCalls += 1
      throw new Error('subscription unavailable')
    })
    const module = createRuntimeModule(mock)

    await expect(module.start()).resolves.toBeUndefined()
    await module.start()
    module.dispose()
    module.dispose()

    expect(mock.subscriptionCalls).toBe(1)
    expect(mock.getStatusCalls).toBe(0)
    expect(mock.stopCalls).toBe(0)
    expect(module.state).toMatchObject({status: 'problem', snapshot: null, problem: {code: 'internal'}})
  })

  it('retains its last snapshot and marks it stale when a later refresh fails', async () => {
    const mock = new RuntimeMock()
    const module = await startWith(mock, runtimeWire(7, 'avtr_kept'))
    const refreshed = module.refresh()

    mock.rejectPending(1, new Error('transport unavailable'))
    await refreshed

    expect(module.state).toMatchObject({
      status: 'stale',
      revision: 7,
      updatedAt: '2026-08-31T00:00:07Z',
      snapshot: {avatar: {id: 'avtr_kept'}},
      problem: {code: 'internal', tone: 'danger'},
    })
  })

  const oscCases: ReadonlyArray<readonly [Partial<RuntimeOscWire>, string]> = [
    [{running: false}, 'not_running'],
    [{running: true, connected: false, hasTarget: false}, 'discovering'],
    [{running: true, connected: true, hasTarget: true, targetMode: 'auto'}, 'discovered'],
    [{running: true, connected: true, hasTarget: true, targetMode: 'manual'}, 'manual'],
    [{running: true, connected: true, hasTarget: true, lastError: 'discovery unavailable'}, 'error'],
  ]

  it.each(oscCases)('maps bounded OSC status to %s without inventing listener ports', async (osc, state) => {
    const mock = new RuntimeMock()
    const module = await startWith(mock, runtimeWire(5, 'avtr_current', {
      osc: {
        ...defaultOscWire(),
        ...osc,
      },
    }))

    const view = module.state.snapshot
    if (view?.osc === undefined) {
      throw new Error('expected mapped OSC view')
    }

    expect(view.osc).toEqual({
      state,
      target: osc.hasTarget === false ? undefined : {host: '192.0.2.5', port: 9001},
      error: osc.lastError,
    })
    expect(Object.isFrozen(view)).toBe(true)
    if (osc.hasTarget !== false) {
      expect(Object.isFrozen(view.osc.target)).toBe(true)
    }
  })

  it('stops its only event subscription exactly once when disposed twice', async () => {
    const mock = new RuntimeMock()
    const module = await startWith(mock, runtimeWire(1))

    module.dispose()
    module.dispose()
    mock.emit({revision: 2})

    expect(mock.stopCalls).toBe(1)
    expect(mock.getStatusCalls).toBe(1)
  })

  it.each(['success', 'failure'] as const)('does not commit a late %s after disposal', async (outcome) => {
    const mock = new RuntimeMock()
    const module = createRuntimeModule(mock)
    const unhandled = vi.fn((event: PromiseRejectionEvent) => event.preventDefault())
    window.addEventListener('unhandledrejection', unhandled)
    try {
      const starting = module.start()
      const before = runtimeState(module)
      module.dispose()

      if (outcome === 'success') mock.resolvePending(0, runtimeWire(1, 'late-avatar'))
      else mock.rejectPending(0, new Error('late Runtime failure'))
      await starting
      await Promise.resolve()

      expect(runtimeState(module)).toEqual(before)
      expect(mock.stopCalls).toBe(1)
      expect(unhandled).not.toHaveBeenCalled()
    } finally {
      window.removeEventListener('unhandledrejection', unhandled)
    }
  })
})

function defaultOscWire(): RuntimeOscWire {
  return {
    running: true,
    connected: true,
    hasTarget: true,
    targetMode: 'auto',
    target: {host: '192.0.2.5', port: 9001},
  }
}

function runtimeState(module: ReturnType<typeof createRuntimeModule>) {
  return {
    status: module.state.status,
    snapshot: module.state.snapshot,
    revision: module.state.revision,
    updatedAt: module.state.updatedAt,
    problem: module.state.problem,
  }
}
