import {presentProblem, type ProblemView} from '../../presentation/problem.js'
import type {RuntimePort, Stop} from '../../wails/ports.js'

import {acceptRevision} from '../shared/revision.js'
import {mapRuntimeWire} from './map.js'
import type {RuntimeModule, RuntimeModuleState, RuntimeView} from './types.js'

interface MutableRuntimeModuleState {
  status: RuntimeModuleState['status']
  snapshot: RuntimeView | null
  revision: number | null
  updatedAt: string | null
  problem: ProblemView | null
}

export function createRuntimeModule(port: RuntimePort): RuntimeModule {
  const state = $state<MutableRuntimeModuleState>({
    status: 'loading',
    snapshot: null,
    revision: null,
    updatedAt: null,
    problem: null,
  })
  let started = false
  let disposed = false
  let startPromise: Promise<void> | null = null
  let stop: Stop | null = null
  let nextRequest = 0

  const refresh = async () => {
    if (disposed) {
      return
    }

    const request = nextRequest + 1
    nextRequest = request

    try {
      const wire = await port.getStatus()
      if (disposed || request !== nextRequest) {
        return
      }

      const currentRevision = state.revision ?? -1
      if (!acceptRevision(currentRevision, wire.revision)) {
        rejectInvalidRevision(request)
        return
      }

      state.snapshot = mapRuntimeWire(wire)
      state.revision = wire.revision
      state.updatedAt = wire.updatedAt
      state.problem = wire.problem === null || wire.problem === undefined
        ? null
        : presentProblem(wire.problem)
      state.status = state.problem === null ? 'ready' : 'problem'
    } catch {
      if (disposed || request !== nextRequest) {
        return
      }

      state.problem = presentProblem({code: 'internal', message: ''})
      state.status = state.snapshot === null ? 'problem' : 'stale'
    }
  }

  return {
    state,
    start() {
      if (disposed) {
        return Promise.resolve()
      }

      if (started) {
        return startPromise ?? Promise.resolve()
      }

      started = true
      stop = port.onChanged(() => {
        void refresh()
      })
      startPromise = refresh()
      return startPromise
    },
    refresh,
    dispose() {
      if (disposed) {
        return
      }

      disposed = true
      const currentStop = stop
      stop = null
      currentStop?.()
    },
  }

  function rejectInvalidRevision(request: number) {
    if (request !== nextRequest) {
      return
    }

    state.problem = presentProblem({code: 'internal', message: ''})
    state.status = state.snapshot === null ? 'problem' : 'stale'
  }
}
