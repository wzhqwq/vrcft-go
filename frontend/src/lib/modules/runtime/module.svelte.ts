import {presentProblem, type ProblemView} from '../../presentation/problem.js'
import {diagnosticText, diagnosticLogPath} from '../../presentation/diagnostics.js'
import type {DiagnosticsWire} from '../../wails/types.js'
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
  const diagnostics = $state<{snapshot: DiagnosticsWire | null; loading: boolean; error: string | null}>({snapshot: null, loading: false, error: null})
  let diagnosticRequest = 0

  const refreshDiagnostics = async () => {
    if (disposed || !port.getDiagnostics) return
    const request = ++diagnosticRequest
    diagnostics.loading = true
    try {
      const wire = await port.getDiagnostics()
      if (disposed || request !== diagnosticRequest) return
      const entry = (value: DiagnosticsWire['entries'][number]) => ({
        id: diagnosticText(value.id, 100), time: diagnosticText(value.time, 100),
        level: diagnosticText(value.level, 16), component: diagnosticText(value.component, 100),
        stage: diagnosticText(value.stage, 100), message: diagnosticText(value.message),
      })
      diagnostics.snapshot = {
        entries: (wire.entries ?? []).slice(-200).map(entry),
        failure: wire.failure ? entry(wire.failure) : null,
        logPath: diagnosticLogPath(wire.logPath), diskError: diagnosticText(wire.diskError),
      }
      diagnostics.error = null
    } catch (error) {
      if (!disposed && request === diagnosticRequest) diagnostics.error = `读取日志失败：${diagnosticText(error)}`
    } finally {
      if (!disposed && request === diagnosticRequest) diagnostics.loading = false
    }
  }

  const refresh = async () => {
    if (disposed) {
      return
    }

    const request = nextRequest + 1
    nextRequest = request

    let stage = 'get_status'
    try {
      const wire = await port.getStatus()
      if (disposed || request !== nextRequest) {
        return
      }

      stage = 'parse_status'
      const currentRevision = state.revision ?? -1
      if (!acceptRevision(currentRevision, wire.revision)) {
        fail('revision', '收到无效或早于当前状态的修订号')
        return
      }

      stage = 'parse_status'
      state.snapshot = mapRuntimeWire(wire)
      state.revision = wire.revision
      state.updatedAt = wire.updatedAt
      state.problem = wire.problem === null || wire.problem === undefined
        ? null
        : presentProblem(wire.problem)
      state.status = state.problem === null ? 'ready' : 'problem'
    } catch (error) {
      if (disposed || request !== nextRequest) {
        return
      }

      fail(stage, error)
    }
  }

  return {
    state,
    diagnostics,
    refreshDiagnostics,
    start() {
      if (disposed) {
        return Promise.resolve()
      }

      if (started) {
        return startPromise ?? Promise.resolve()
      }

      started = true
      try {
        stop = port.onChanged(() => {
          void refresh()
        })
      } catch (error) {
        fail('subscribe', error)
        startPromise = Promise.resolve()
        return startPromise
      }
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

  function fail(stage: string, error: unknown) {
    const message = diagnosticText(error)
    const labels: Record<string, string> = {get_status: '状态请求失败', parse_status: '状态解析失败', revision: '状态修订异常', subscribe: '事件订阅失败'}
    state.problem = presentProblem({code: 'internal', message: `${labels[stage]} (${stage})：${message}`})
    state.status = state.snapshot === null ? 'problem' : 'stale'
    try { void port.reportFrontendError?.(stage, message).catch(() => undefined) } catch { /* Reporting must not break status handling. */ }
  }
}
