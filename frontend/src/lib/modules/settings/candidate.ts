import type {SettingsCandidate as SettingsCandidateWire} from '../../wails/types.js'
import type {SettingsCandidate} from './form.js'
import type {DeepReadonly} from './types.js'

export function cloneCandidate(value: DeepReadonly<SettingsCandidateWire>): SettingsCandidate {
  return {
    avatar: {...value.avatar},
    plugins: {devRoots: [...(value.plugins.devRoots ?? [])]},
    processing: {
      defaultChannel: cloneChannel(value.processing.defaultChannel),
      overrides: (value.processing.overrides ?? []).map((override) => ({
        name: override.name,
        channel: cloneChannel(override.channel),
      })),
      activeStaleAfterMs: value.processing.activeStaleAfterMs,
      mutualExclusion: (value.processing.mutualExclusion ?? []).map((group) => [...(group ?? [])]),
    },
    osc: {...value.osc},
  }
}

export function immutableCandidate(value: DeepReadonly<SettingsCandidateWire>): DeepReadonly<SettingsCandidate> {
  return deepFreeze(cloneCandidate(value))
}

export function sameCandidate(left: SettingsCandidate, right: SettingsCandidate): boolean {
  return semanticEqual(left, right)
}

function cloneChannel(channel: SettingsCandidate['processing']['defaultChannel']) {
  return {
    calibration: {...channel.calibration},
    tuning: {...channel.tuning},
    filter: {...channel.filter},
    dropout: {...channel.dropout},
  }
}

function deepFreeze<T>(value: T): T {
  if (value !== null && typeof value === 'object' && !Object.isFrozen(value)) {
    for (const nested of Object.values(value)) deepFreeze(nested)
    Object.freeze(value)
  }
  return value
}

function semanticEqual(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true
  if (left === null || right === null || typeof left !== 'object' || typeof right !== 'object') return false
  if (Array.isArray(left) || Array.isArray(right)) {
    if (!Array.isArray(left) || !Array.isArray(right) || left.length !== right.length) return false
    return left.every((value, index) => semanticEqual(value, right[index]))
  }
  const leftRecord = left as Record<string, unknown>
  const rightRecord = right as Record<string, unknown>
  const keys = Object.keys(leftRecord)
  return keys.length === Object.keys(rightRecord).length
    && keys.every((key) => Object.hasOwn(rightRecord, key) && semanticEqual(leftRecord[key], rightRecord[key]))
}
