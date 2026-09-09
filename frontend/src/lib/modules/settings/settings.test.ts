import {describe, expect, it, vi} from 'vitest'

import {createSettingsModule, fieldTargets, validateCandidate} from './index.js'
import {cloneCandidate} from './candidate.js'
import type {SettingsPort, Stop} from '../../wails/ports.js'
import type {
  ProcessingChannelWire,
  ProblemWire,
  SettingsCandidate,
  SettingsSaveWire,
  SettingsValidationWire,
  SettingsWire,
} from '../../wails/types.js'

class SettingsMock implements SettingsPort {
  readonly listeners = new Set<(value: unknown) => void>()
  readonly getPending: Array<Deferred<SettingsWire>> = []
  readonly validationPending: Array<Deferred<SettingsValidationWire>> = []
  readonly savePending: Array<Deferred<SettingsSaveWire>> = []
  readonly validateCalls: SettingsCandidate[] = []
  readonly saveCalls: Array<{expectedRevision: number; candidate: SettingsCandidate}> = []
  getCalls = 0
  subscriptionCalls = 0
  stopCalls = 0

  get(): Promise<SettingsWire> {
    this.getCalls += 1
    return deferredInto(this.getPending)
  }

  validate(candidate: SettingsCandidate): Promise<SettingsValidationWire> {
    this.validateCalls.push(candidate)
    return deferredInto(this.validationPending)
  }

  save(expectedRevision: number, candidate: SettingsCandidate): Promise<SettingsSaveWire> {
    this.saveCalls.push({expectedRevision, candidate})
    return deferredInto(this.savePending)
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

function channel(): ProcessingChannelWire {
  return {
    calibration: {enabled: true, neutral: 0, min: -1, max: 1, gain: 1, invert: false},
    tuning: {deadzone: 0, gain: 1, exponent: 1, clampEnabled: true, clampMin: -1, clampMax: 1},
    filter: {mode: 'none', emaAlpha: 0.5, minCutoff: 1, beta: 0, derivativeCutoff: 1},
    dropout: {holdDurationMs: 0, decayDurationMs: 0, staleAfterMs: 1000},
  }
}

function candidate(overrides: Partial<SettingsCandidate> = {}): SettingsCandidate {
  return {
    avatar: {oscRoot: 'C:\\VRChat\\OSC', fallbackPath: ''},
    plugins: {devRoots: ['C:\\plugins']},
    processing: {
      defaultChannel: channel(),
      overrides: [{name: 'eye.left_gaze_x', channel: channel()}],
      activeStaleAfterMs: 1000,
      mutualExclusion: [['eye.left_gaze_x', 'eye.right_gaze_x']],
    },
    osc: {targetMode: 'auto', preferredService: '', manualHost: '', manualPort: 0},
    ...overrides,
  }
}

function getWire(revision: number, settings = candidate(), problem?: ProblemWire): SettingsWire {
  return {revision, updatedAt: `2026-09-01T00:00:${String(revision).padStart(2, '0')}Z`, fileRevision: revision, settings, problem}
}

function validationWire(revision: number, settings: SettingsCandidate, problem?: ProblemWire): SettingsValidationWire {
  return {revision, updatedAt: `2026-09-01T00:01:${String(revision).padStart(2, '0')}Z`, settings, problem}
}

function saveWire(revision: number, settings: SettingsCandidate, restartRequired: boolean, problem?: ProblemWire): SettingsSaveWire {
  return {revision, updatedAt: `2026-09-01T00:02:${String(revision).padStart(2, '0')}Z`, fileRevision: revision, settings, restartRequired, problem}
}

async function startWith(mock: SettingsMock, wire: SettingsWire) {
  const module = createSettingsModule(mock)
  const started = module.start()
  mock.getPending[0]?.resolve(wire)
  await started
  return module
}

describe('settings candidates and fields', () => {
  it('defines only the approved backend field paths and validates client constraints', () => {
    expect(fieldTargets).toEqual({
      'avatar.oscRoot': {section: 'general', control: 'avatar-osc-root'},
      'avatar.fallbackPath': {section: 'general', control: 'avatar-fallback-path'},
      'plugins.devRoots': {section: 'general', control: 'plugin-dev-roots'},
      'processing': {section: 'processing', control: 'processing-summary'},
      'processing.defaultChannel': {section: 'processing', control: 'default-channel'},
      'processing.activeStaleAfterMs': {section: 'processing', control: 'active-stale-after'},
      'processing.overrides': {section: 'processing', control: 'channel-overrides'},
      'processing.mutualExclusion': {section: 'processing', control: 'mutual-exclusion'},
      'osc.targetMode': {section: 'osc', control: 'osc-target-mode'},
      'osc.preferredService': {section: 'osc', control: 'osc-preferred-service'},
      'osc.manualHost': {section: 'osc', control: 'osc-manual-host'},
      'osc.manualPort': {section: 'osc', control: 'osc-manual-port'},
    })

    const invalid = candidate({
      avatar: {oscRoot: ' ', fallbackPath: ''},
      plugins: {devRoots: ['C:\\one', ' ']},
      processing: {...candidate().processing, overrides: [{name: 'eye', channel: channel()}, {name: ' eye ', channel: channel()}]},
      osc: {targetMode: 'manual', preferredService: 'VRChat-Client', manualHost: ' ', manualPort: 65536},
    })
    invalid.processing.defaultChannel.tuning.gain = Number.NaN
    invalid.processing.activeStaleAfterMs = Number.NaN

    expect([...validateCandidate(cloneCandidate(invalid)).keys()].sort()).toEqual([
      'avatar.oscRoot', 'osc.manualHost', 'osc.manualPort', 'osc.preferredService', 'plugins.devRoots',
      'processing.activeStaleAfterMs', 'processing.defaultChannel', 'processing.overrides',
    ])
  })
})

describe('Settings module', () => {
  it('normalizes Go nil collections before owning and editing the candidate', async () => {
    const source = candidate()
    Object.assign(source.plugins, {devRoots: null})
    Object.assign(source.processing, {overrides: null, mutualExclusion: null})
    const mock = new SettingsMock()
    const module = await startWith(mock, getWire(1, source))
    expect(module.state.status).toBe('ready')
    expect(module.state.draft?.plugins.devRoots).toEqual([])
    expect(module.state.draft?.processing.overrides).toEqual([])
    expect(module.state.draft?.processing.mutualExclusion).toEqual([])
    module.updateDraft((draft) => { draft.plugins.devRoots.push('C:\\plugins') })
    expect(module.state.server?.plugins.devRoots).toEqual([])
    expect(module.state.dirty).toBe(true)
    const saving = module.save()
    mock.validationPending[0]?.resolve(validationWire(1, source))
    await vi.waitFor(() => expect(mock.saveCalls).toHaveLength(1))
    expect(mock.saveCalls[0]?.candidate.processing.mutualExclusion).toEqual([])
    mock.savePending[0]?.resolve(saveWire(2, source, true))
    expect(await saving).toBe(true)
    expect(module.state.draft?.processing.mutualExclusion).toEqual([])
  })

  it('owns nil mutual-exclusion rows as editable empty groups', async () => {
    const source = candidate()
    Object.assign(source.processing, {mutualExclusion: [null, ['eye']]})
    const module = await startWith(new SettingsMock(), getWire(1, source))
    expect(module.state.draft?.processing.mutualExclusion).toEqual([[], ['eye']])
  })

  it('owns a synchronous subscription failure as an idempotent no-data problem', async () => {
    const mock = new SettingsMock()
    vi.spyOn(mock, 'onChanged').mockImplementation(() => {
      mock.subscriptionCalls += 1
      throw new Error('subscription unavailable')
    })
    const module = createSettingsModule(mock)

    await expect(module.start()).resolves.toBeUndefined()
    await module.start()
    module.dispose()
    module.dispose()

    expect(mock.subscriptionCalls).toBe(1)
    expect(mock.getCalls).toBe(0)
    expect(mock.stopCalls).toBe(0)
    expect(module.state).toMatchObject({status: 'problem', server: null, draft: null, problem: {code: 'internal'}})
  })

  it('deep-clones immutable server/draft state and computes semantic dirty equality', async () => {
    const mock = new SettingsMock()
    const source = candidate()
    const module = await startWith(mock, getWire(2, source))

    source.plugins.devRoots![0] = 'mutated upstream'
    expect(() => (module.state.server!.plugins.devRoots as string[]).push('injected')).toThrow()
    expect(() => ((module.state.draft!.processing.overrides[0] as {name: string}).name = 'injected')).toThrow()
    expect(module.state.server?.plugins.devRoots).toEqual(['C:\\plugins'])

    module.updateDraft((draft) => { draft.osc.manualPort = 9001 })
    expect(module.state.dirty).toBe(true)
    expect(module.state.server?.osc.manualPort).toBe(0)
    module.updateDraft((draft) => { draft.osc.manualPort = 0 })
    expect(module.state.dirty).toBe(false)
  })

  it('uses one idempotent event subscription and treats payloads only as Settings invalidation hints', async () => {
    const mock = new SettingsMock()
    const module = await startWith(mock, getWire(1))
    await module.start()
    mock.emit(getWire(99, candidate({avatar: {oscRoot: 'payload', fallbackPath: ''}})))

    expect(mock.subscriptionCalls).toBe(1)
    expect(mock.getCalls).toBe(2)
    mock.getPending[1]?.resolve(getWire(2, candidate({avatar: {oscRoot: 'queried', fallbackPath: ''}})))
    await vi.waitFor(() => expect(module.state.server?.avatar.oscRoot).toBe('queried'))

    module.dispose()
    module.dispose()
    mock.emit({revision: 3})
    expect(mock.stopCalls).toBe(1)
    expect(mock.getCalls).toBe(2)
  })

  it('fences superseded refresh success, failure, and unsafe revisions', async () => {
    const mock = new SettingsMock()
    const module = await startWith(mock, getWire(3))
    const old = module.refresh()
    const latest = module.refresh()
    mock.getPending[2]?.resolve(getWire(4, candidate({avatar: {oscRoot: 'latest', fallbackPath: ''}})))
    await latest
    mock.getPending[1]?.resolve(getWire(5, candidate({avatar: {oscRoot: 'late', fallbackPath: ''}})))
    await old
    expect(module.state.server?.avatar.oscRoot).toBe('latest')

    const superseded = module.refresh()
    const current = module.refresh()
    mock.getPending[3]?.reject(new Error('old failure'))
    await superseded
    expect(module.state.status).toBe('ready')
    mock.getPending[4]?.resolve(getWire(Number.MAX_SAFE_INTEGER + 1))
    await current
    expect(module.state).toMatchObject({status: 'stale', revision: 4})
  })

  it('preserves a dirty draft on event refresh and exposes conflict until explicit reload', async () => {
    const mock = new SettingsMock()
    const module = await startWith(mock, getWire(4))
    module.updateDraft((draft) => { draft.avatar.fallbackPath = 'C:\\mine.json' })
    mock.emit({})
    mock.getPending[1]?.resolve(getWire(5, candidate({avatar: {oscRoot: 'C:\\VRChat\\OSC', fallbackPath: 'C:\\theirs.json'}})))
    await vi.waitFor(() => expect(module.state.revision).toBe(5))

    expect(module.state.draft?.avatar.fallbackPath).toBe('C:\\mine.json')
    expect(module.state.server?.avatar.fallbackPath).toBe('C:\\theirs.json')
    expect(module.state.conflict).toBe(true)

    const reloading = module.reload()
    mock.getPending[2]?.resolve(getWire(6, candidate({avatar: {oscRoot: 'C:\\VRChat\\OSC', fallbackPath: 'C:\\loaded.json'}})))
    await reloading
    expect(module.state).toMatchObject({dirty: false, conflict: false})
    expect(module.state.draft?.avatar.fallbackPath).toBe('C:\\loaded.json')
  })

  it('performs field/backend validation with latest-issued fencing and retains unknown Problems', async () => {
    const mock = new SettingsMock()
    const module = await startWith(mock, getWire(2))
    const first = module.validate('avatar.oscRoot')
    const second = module.validate('avatar.oscRoot')
    mock.validationPending[1]?.resolve(validationWire(2, candidate(), {code: 'validation', message: 'new', field: 'avatar.oscRoot'}))
    await second
    mock.validationPending[0]?.reject(new Error('old transport'))
    await first
    expect(module.state.fieldProblems.get('avatar.oscRoot')).toMatchObject({detail: 'new'})

    const unknown = module.validate()
    mock.validationPending[2]?.resolve(validationWire(2, candidate(), {code: 'validation', message: 'general', field: 'future.field'}))
    await unknown
    expect(module.state.problem).toMatchObject({detail: 'general', field: 'future.field'})
  })

  it.each([
    ['absent', {code: 'validation', message: 'general failure'}],
    ['unknown', {code: 'validation', message: 'future failure', field: 'future.field'}],
  ] as const)('keeps a backend Problem with an %s field general during field validation', async (_name, problem) => {
    const mock = new SettingsMock()
    const module = await startWith(mock, getWire(2))
    const validating = module.validate('avatar.oscRoot')
    mock.validationPending[0]?.resolve(validationWire(2, candidate(), problem))

    expect(await validating).toBe(false)
    expect(module.state.fieldProblems.has('avatar.oscRoot')).toBe(false)
    expect(module.state.problem).toMatchObject({detail: problem.message, field: 'field' in problem ? problem.field : undefined})
  })

  it('clears old draft field Problems on clean replacement but preserves them with a dirty draft', async () => {
    const cleanMock = new SettingsMock()
    const clean = await startWith(cleanMock, getWire(2))
    const cleanValidation = clean.validate('avatar.oscRoot')
    cleanMock.validationPending[0]?.resolve(validationWire(2, candidate(), {
      code: 'validation', message: 'old clean error', field: 'avatar.oscRoot',
    }))
    await cleanValidation
    expect(clean.state.fieldProblems.has('avatar.oscRoot')).toBe(true)
    const cleanRefresh = clean.refresh()
    cleanMock.getPending[1]?.resolve(getWire(3, candidate({avatar: {oscRoot: 'C:\\fresh', fallbackPath: ''}})))
    await cleanRefresh
    expect(clean.state.fieldProblems.size).toBe(0)

    const dirtyMock = new SettingsMock()
    const dirty = await startWith(dirtyMock, getWire(2))
    dirty.updateDraft((draft) => { draft.avatar.fallbackPath = 'C:\\mine.json' })
    const dirtyValidation = dirty.validate('avatar.oscRoot')
    dirtyMock.validationPending[0]?.resolve(validationWire(2, candidate(), {
      code: 'validation', message: 'draft error', field: 'avatar.oscRoot',
    }))
    await dirtyValidation
    const dirtyRefresh = dirty.refresh()
    dirtyMock.getPending[1]?.resolve(getWire(3, candidate({avatar: {oscRoot: 'C:\\fresh', fallbackPath: ''}})))
    await dirtyRefresh
    expect(dirty.state.fieldProblems.get('avatar.oscRoot')).toMatchObject({detail: 'draft error'})
    expect(dirty.state.draft?.avatar.fallbackPath).toBe('C:\\mine.json')
  })

  it('ignores a stale validation response after an authoritative dirty-draft refresh', async () => {
    const mock = new SettingsMock()
    const module = await startWith(mock, getWire(2))
    module.updateDraft((draft) => { draft.avatar.fallbackPath = 'C:\\mine.json' })
    const validating = module.validate('avatar.oscRoot')
    mock.emit({})
    mock.getPending[1]?.resolve(getWire(3, candidate({avatar: {oscRoot: 'C:\\new-server', fallbackPath: ''}})))
    await vi.waitFor(() => expect(module.state.revision).toBe(3))
    mock.validationPending[0]?.resolve(validationWire(2, candidate(), {
      code: 'validation', message: 'stale validation', field: 'avatar.oscRoot',
    }))

    expect(await validating).toBe(false)
    expect(module.state).toMatchObject({revision: 3, problem: null, conflict: true})
    expect(module.state.fieldProblems.size).toBe(0)
    expect(module.state.server?.avatar.oscRoot).toBe('C:\\new-server')
    expect(module.state.draft?.avatar.fallbackPath).toBe('C:\\mine.json')
  })

  it('blocks invalid or pending saves and revalidates before saving the exact owned candidate', async () => {
    const mock = new SettingsMock()
    const module = await startWith(mock, getWire(7))
    module.updateDraft((draft) => { draft.avatar.oscRoot = '' })
    expect(await module.save()).toBe(false)
    expect(mock.validateCalls).toHaveLength(0)

    module.updateDraft((draft) => { draft.avatar.oscRoot = 'C:\\VRChat\\OSC'; draft.osc.manualPort = 9001 })
    const saving = module.save()
    expect(module.state.saving).toBe(true)
    expect(module.save()).toBe(saving)
    mock.validationPending[0]?.resolve(validationWire(7, mock.validateCalls[0]!))
    await vi.waitFor(() => expect(mock.saveCalls).toHaveLength(1))
    expect(mock.saveCalls[0]).toEqual({expectedRevision: 7, candidate: expect.objectContaining({osc: expect.objectContaining({manualPort: 9001})})})
    mock.savePending[0]?.resolve(saveWire(8, mock.saveCalls[0]!.candidate, true))
    expect(await saving).toBe(true)
    expect(module.state).toMatchObject({dirty: false, saving: false, restartRequired: true})
  })

  it('does not persist a superseded candidate when the draft changes during save validation', async () => {
    const mock = new SettingsMock()
    const module = await startWith(mock, getWire(7))
    module.updateDraft((draft) => { draft.avatar.fallbackPath = 'C:\\first-edit.json' })
    const saving = module.save()
    expect(mock.validateCalls[0]?.avatar.fallbackPath).toBe('C:\\first-edit.json')

    module.updateDraft((draft) => { draft.avatar.fallbackPath = 'C:\\newer-edit.json' })
    mock.validationPending[0]?.resolve(validationWire(7, mock.validateCalls[0]!))

    expect(await saving).toBe(false)
    expect(mock.saveCalls).toHaveLength(0)
    expect(module.state.draft?.avatar.fallbackPath).toBe('C:\\newer-edit.json')
    expect(module.state).toMatchObject({dirty: true, saving: false})
  })

  it('preserves the draft on backend validation/save conflict and on save failure', async () => {
    const mock = new SettingsMock()
    const module = await startWith(mock, getWire(3))
    module.updateDraft((draft) => { draft.avatar.fallbackPath = 'C:\\mine.json' })
    const saving = module.save()
    mock.validationPending[0]?.resolve(validationWire(3, mock.validateCalls[0]!))
    await vi.waitFor(() => expect(mock.saveCalls).toHaveLength(1))
    mock.savePending[0]?.resolve(saveWire(4, candidate(), false, {code: 'conflict', message: 'changed', currentRevision: 4}))
    expect(await saving).toBe(false)
    expect(module.state.draft?.avatar.fallbackPath).toBe('C:\\mine.json')
    expect(module.state).toMatchObject({dirty: true, conflict: true})

    const retry = module.save()
    mock.validationPending[1]?.resolve(validationWire(3, mock.validateCalls[1]!))
    await vi.waitFor(() => expect(mock.saveCalls).toHaveLength(2))
    mock.savePending[1]?.reject(new Error('offline'))
    expect(await retry).toBe(false)
    expect(module.state.draft?.avatar.fallbackPath).toBe('C:\\mine.json')
  })

  it('preserves restart-required state across refreshes and newer edits during save', async () => {
    const mock = new SettingsMock()
    const module = await startWith(mock, getWire(10))
    module.updateDraft((draft) => { draft.plugins.devRoots = ['C:\\new-plugin-root'] })
    const saving = module.save()
    mock.validationPending[0]?.resolve(validationWire(10, mock.validateCalls[0]!))
    await vi.waitFor(() => expect(mock.saveCalls).toHaveLength(1))
    module.updateDraft((draft) => { draft.avatar.fallbackPath = 'C:\\newer-edit.json' })
    mock.savePending[0]?.resolve(saveWire(11, mock.saveCalls[0]!.candidate, true))
    expect(await saving).toBe(true)
    expect(module.state.restartRequired).toBe(true)
    expect(module.state.draft?.avatar.fallbackPath).toBe('C:\\newer-edit.json')
    expect(module.state.dirty).toBe(true)

    const refresh = module.refresh()
    mock.getPending[1]?.resolve(getWire(11, mock.saveCalls[0]!.candidate))
    await refresh
    expect(module.state.restartRequired).toBe(true)
    expect(module.state.draft?.avatar.fallbackPath).toBe('C:\\newer-edit.json')
  })

  it.each(['success', 'failure'] as const)('does not commit a late %s after disposal', async (outcome) => {
    const mock = new SettingsMock()
    const module = createSettingsModule(mock)
    const unhandled = vi.fn((event: PromiseRejectionEvent) => event.preventDefault())
    window.addEventListener('unhandledrejection', unhandled)
    try {
      const starting = module.start()
      const before = {
        status: module.state.status, server: module.state.server, draft: module.state.draft,
        revision: module.state.revision, fileRevision: module.state.fileRevision,
        updatedAt: module.state.updatedAt, problem: module.state.problem,
      }
      module.dispose()

      if (outcome === 'success') mock.getPending[0]?.resolve(getWire(1))
      else mock.getPending[0]?.reject(new Error('late Settings failure'))
      await starting
      await Promise.resolve()

      expect({
        status: module.state.status, server: module.state.server, draft: module.state.draft,
        revision: module.state.revision, fileRevision: module.state.fileRevision,
        updatedAt: module.state.updatedAt, problem: module.state.problem,
      }).toEqual(before)
      expect(mock.stopCalls).toBe(1)
      expect(unhandled).not.toHaveBeenCalled()
    } finally {
      window.removeEventListener('unhandledrejection', unhandled)
    }
  })
})
