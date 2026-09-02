import type {RuntimeOscWire, RuntimeWire} from '../../wails/types.js'

import type {OscState, RuntimeOscView, RuntimeView} from './types.js'

export function mapRuntimeWire(wire: RuntimeWire): RuntimeView {
  const application = wire.application ?? undefined

  return freezeRuntimeView({
    phase: wire.phase,
    platformSupported: wire.platformSupported,
    avatar: {id: application?.avatarId ?? '', name: application?.avatarName ?? ''},
    lifecycle: application?.lifecycle,
    plan: application === undefined ? undefined : {
      generation: application.planGeneration,
      status: application.planStatus,
      source: application.planSource,
      configPath: application.configPath,
      configId: application.configId,
      generationExhausted: application.generationExhausted,
    },
    osc: application === undefined ? undefined : mapOsc(application.osc),
    pluginFailures: application === undefined
      ? []
      : application.pluginFailures.map((failure) => ({
          pluginId: failure.pluginId,
          operation: failure.operation,
          message: failure.message,
        })),
    planError: application?.planError,
    runtimeError: application?.runtimeError,
  })
}

export function mapOsc(osc: RuntimeOscWire): RuntimeOscView {
  return {
    state: classifyOsc(osc),
    target: osc.hasTarget ? {host: osc.target.host, port: osc.target.port} : undefined,
    error: osc.lastError || undefined,
  }
}

export function classifyOsc(osc: RuntimeOscWire): OscState {
  if (!osc.running) {
    return 'not_running'
  }

  if (osc.lastError) {
    return 'error'
  }

  if (osc.targetMode === 'manual' && osc.hasTarget) {
    return 'manual'
  }

  if (osc.connected && osc.hasTarget) {
    return 'discovered'
  }

  return 'discovering'
}

function freezeRuntimeView(view: RuntimeView): RuntimeView {
  const avatar = Object.freeze({...view.avatar})
  const plan = view.plan === undefined ? undefined : Object.freeze({...view.plan})
  const osc = view.osc === undefined
    ? undefined
    : Object.freeze({
        ...view.osc,
        target: view.osc.target === undefined ? undefined : Object.freeze({...view.osc.target}),
      })
  const pluginFailures = Object.freeze(view.pluginFailures.map((failure) => Object.freeze({...failure})))

  return Object.freeze({...view, avatar, plan, osc, pluginFailures})
}
