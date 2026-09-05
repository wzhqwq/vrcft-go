import {presentProblem, type ProblemView} from '../../presentation/problem.js'
import type {SettingsPort, Stop} from '../../wails/ports.js'
import type {ProblemWire, SettingsCandidate, SettingsWire} from '../../wails/types.js'
import {acceptRevision} from '../shared/revision.js'
import {cloneCandidate, immutableCandidate, sameCandidate} from './candidate.js'
import {isSettingsField, validateCandidate, type SettingsField} from './fields.js'
import type {DeepReadonly, SettingsModule, SettingsModuleState} from './types.js'

interface MutableState {
  status: SettingsModuleState['status']
  server: DeepReadonly<SettingsCandidate> | null
  draft: DeepReadonly<SettingsCandidate> | null
  revision: number | null
  fileRevision: number | null
  updatedAt: string | null
  problem: ProblemView | null
  fieldProblems: Array<[SettingsField, ProblemView]>
  validating: boolean
  saving: boolean
  restartRequired: boolean
  conflict: boolean
}

export function createSettingsModule(port: SettingsPort): SettingsModule {
  const data = $state<MutableState>({
    status: 'loading', server: null, draft: null, revision: null, fileRevision: null,
    updatedAt: null, problem: null, fieldProblems: [], validating: false, saving: false,
    restartRequired: false, conflict: false,
  })
  let started = false
  let disposed = false
  let startPromise: Promise<void> | null = null
  let stop: Stop | null = null
  let nextLoad = 0
  let nextValidation = 0
  let draftVersion = 0
  let savePromise: Promise<boolean> | null = null

  const state: SettingsModuleState = {
    get status() { return data.status },
    get server() { return data.server },
    get draft() { return data.draft },
    get revision() { return data.revision },
    get fileRevision() { return data.fileRevision },
    get updatedAt() { return data.updatedAt },
    get problem() { return data.problem },
    get fieldProblems() { return new Map(data.fieldProblems) },
    get dirty() { return dirty() },
    get validating() { return data.validating },
    get saving() { return data.saving },
    get restartRequired() { return data.restartRequired },
    get conflict() { return data.conflict },
  }

  const refresh = () => load(false).then(() => undefined)
  const module: SettingsModule = {
    state,
    start() {
      if (disposed) return Promise.resolve()
      if (started) return startPromise ?? Promise.resolve()
      started = true
      try {
        stop = port.onChanged(() => { void refresh() })
      } catch {
        markLoadFailure()
        startPromise = Promise.resolve()
        return startPromise
      }
      startPromise = refresh()
      return startPromise
    },
    refresh,
    dispose() {
      if (disposed) return
      disposed = true
      nextLoad += 1
      nextValidation += 1
      const currentStop = stop
      stop = null
      currentStop?.()
    },
    updateDraft(update) {
      if (disposed || data.draft === null) return
      const next = cloneCandidate(data.draft as SettingsCandidate)
      update(next)
      data.draft = immutableCandidate(next)
      draftVersion += 1
      clearClientProblems()
    },
    validate: validateDraft,
    save() {
      if (savePromise !== null) return savePromise
      const clientProblems = data.draft === null ? new Map<SettingsField, ProblemView>() : validateCandidate(data.draft as SettingsCandidate)
      replaceFieldProblems(clientProblems)
      if (disposed || data.draft === null || data.revision === null || clientProblems.size !== 0) return Promise.resolve(false)
      data.saving = true
      const operation = performSave()
      savePromise = operation
      void operation.finally(() => {
        if (savePromise === operation) savePromise = null
        if (!disposed) data.saving = false
      })
      return operation
    },
    reload() { return load(true) },
  }
  return module

  function dirty(): boolean {
    return data.server !== null && data.draft !== null
      && !sameCandidate(data.server as SettingsCandidate, data.draft as SettingsCandidate)
  }

  async function load(replaceDraft: boolean): Promise<boolean> {
    if (disposed) return false
    const request = ++nextLoad
    try {
      const wire = await port.get()
      if (disposed || request !== nextLoad) return false
      if (!isAcceptableRevision(wire.revision)) {
        markLoadFailure()
        return false
      }
      applyWire(wire, replaceDraft)
      return true
    } catch {
      if (disposed || request !== nextLoad) return false
      markLoadFailure()
      return false
    }
  }

  function applyWire(wire: SettingsWire, replaceDraft: boolean) {
    const previousServer = data.server
    const previousRevision = data.revision
    const wasDirty = dirty()
    const nextServer = immutableCandidate(wire.settings)
    const changedAuthoritatively = previousServer !== null
      && (!sameCandidate(previousServer as SettingsCandidate, nextServer as SettingsCandidate) || wire.revision > (previousRevision ?? -1))
    data.server = nextServer
    data.revision = wire.revision
    data.fileRevision = wire.fileRevision
    data.updatedAt = wire.updatedAt
    if (replaceDraft || data.draft === null || !wasDirty) {
      data.draft = immutableCandidate(wire.settings)
      draftVersion += 1
      data.conflict = false
      data.fieldProblems = []
      data.problem = null
    } else if (changedAuthoritatively) {
      data.conflict = true
    }
    if (wire.problem != null) applyBackendProblem(wire.problem)
    else if (!wasDirty || replaceDraft || data.problem?.code !== 'validation') data.problem = null
    data.status = wire.problem == null ? 'ready' : 'problem'
  }

  async function validateDraft(field?: SettingsField): Promise<boolean> {
    if (disposed || data.draft === null) return false
    const candidate = cloneCandidate(data.draft as SettingsCandidate)
    const local = validateCandidate(candidate)
    if (field === undefined) replaceFieldProblems(local)
    else replaceOneFieldProblem(field, local.get(field))
    if (field === undefined ? local.size !== 0 : local.has(field)) return false

    const request = ++nextValidation
    const version = draftVersion
    data.validating = true
    try {
      const response = await port.validate(candidate)
      if (disposed || request !== nextValidation || version !== draftVersion) return false
      if (!isSafeRevision(response.revision)) {
        data.problem = internalProblem()
        return false
      }
      if (data.revision !== null && response.revision < data.revision) return false
      if (response.problem == null) {
        if (field === undefined) data.fieldProblems = []
        else replaceOneFieldProblem(field, undefined)
        data.problem = null
        return true
      }
      applyValidationProblem(response.problem)
      return false
    } catch {
      if (disposed || request !== nextValidation || version !== draftVersion) return false
      data.problem = internalProblem()
      return false
    } finally {
      if (!disposed && request === nextValidation) data.validating = false
    }
  }

  async function performSave(): Promise<boolean> {
    const draft = cloneCandidate(data.draft as SettingsCandidate)
    const version = draftVersion
    const expectedRevision = data.revision as number
    const validationRequest = ++nextValidation
    data.validating = true
    let normalized: SettingsCandidate
    try {
      const response = await port.validate(cloneCandidate(draft))
      if (disposed || validationRequest !== nextValidation || version !== draftVersion) return false
      if (!isSafeRevision(response.revision) || response.revision !== expectedRevision) {
        data.problem = internalProblem()
        return false
      }
      if (response.problem != null) {
        applyValidationProblem(response.problem)
        return false
      }
      normalized = cloneCandidate(response.settings)
      data.fieldProblems = []
      data.problem = null
    } catch {
      if (!disposed && validationRequest === nextValidation) data.problem = internalProblem()
      return false
    } finally {
      if (!disposed && validationRequest === nextValidation) data.validating = false
    }
    if (disposed || data.revision !== expectedRevision) {
      data.conflict = true
      return false
    }

    try {
      const response = await port.save(expectedRevision, cloneCandidate(normalized))
      if (disposed) return false
      if (!isSafeRevision(response.revision) || response.revision < expectedRevision) {
        data.problem = internalProblem()
        return false
      }
      if (response.problem != null) {
        applyBackendProblem(response.problem)
        if (response.problem.code === 'conflict') data.conflict = true
        return false
      }

      data.restartRequired ||= response.restartRequired
      if (acceptRevision(data.revision ?? -1, response.revision)) {
        data.server = immutableCandidate(response.settings)
        data.revision = response.revision
        data.fileRevision = response.fileRevision
        data.updatedAt = response.updatedAt
        data.problem = null
        data.status = 'ready'
        if (draftVersion === version) {
          data.draft = immutableCandidate(response.settings)
          draftVersion += 1
        }
        data.conflict = false
        data.fieldProblems = []
      }
      return true
    } catch {
      if (!disposed) data.problem = internalProblem()
      return false
    }
  }

  function isAcceptableRevision(revision: number): boolean {
    return isSafeRevision(revision) && acceptRevision(data.revision ?? -1, revision)
  }

  function markLoadFailure() {
    data.problem = internalProblem()
    data.status = data.server === null ? 'problem' : 'stale'
  }

  function applyBackendProblem(problem: ProblemWire | null | undefined) {
    if (problem == null) {
      data.problem = null
      return
    }
    const presented = freezeProblem(presentProblem(problem))
    if (isSettingsField(problem.field)) {
      replaceOneFieldProblem(problem.field, presented)
      data.problem = null
    } else {
      data.problem = presented
    }
    if (problem.code === 'conflict') data.conflict = true
  }

  function applyValidationProblem(problem: ProblemWire) {
    const presented = freezeProblem(presentProblem(problem))
    if (isSettingsField(problem.field)) {
      replaceOneFieldProblem(problem.field, presented)
      data.problem = null
    } else {
      data.problem = presented
    }
    if (problem.code === 'conflict') data.conflict = true
  }

  function replaceFieldProblems(problems: Map<SettingsField, ProblemView>) {
    data.fieldProblems = [...problems]
  }

  function replaceOneFieldProblem(field: SettingsField, problem: ProblemView | undefined) {
    data.fieldProblems = data.fieldProblems.filter(([key]) => key !== field)
    if (problem !== undefined) data.fieldProblems.push([field, problem])
  }

  function clearClientProblems() {
    data.fieldProblems = data.fieldProblems.filter(([, problem]) => problem.code !== 'validation')
  }
}

function isSafeRevision(revision: number): boolean {
  return Number.isSafeInteger(revision) && revision >= 0
}

function internalProblem(): ProblemView {
  return freezeProblem(presentProblem({code: 'internal', message: ''}))
}

function freezeProblem(problem: ProblemView): ProblemView {
  return Object.freeze(problem)
}
