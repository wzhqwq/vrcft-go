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
  it('rejects unsafe and older revisions', () => {
    expect(acceptRevision(3, 3)).toBe(true)
    expect(acceptRevision(3, 4)).toBe(true)
    expect(acceptRevision(3, 2)).toBe(false)
    expect(acceptRevision(3, Number.MAX_SAFE_INTEGER + 1)).toBe(false)
    expect(acceptRevision(3, Number.NaN)).toBe(false)
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
