import {copy} from '../../../copy/zh-CN.js'
import type {ProblemView} from '../../presentation/problem.js'
import type {SettingsCandidate} from './form.js'

export const fieldTargets = {
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
} as const

export type SettingsField = keyof typeof fieldTargets

export function isSettingsField(value: string | undefined): value is SettingsField {
  return value !== undefined && Object.hasOwn(fieldTargets, value)
}

export function validateCandidate(candidate: SettingsCandidate): Map<SettingsField, ProblemView> {
  const problems = new Map<SettingsField, ProblemView>()
  if (candidate.avatar.oscRoot.trim() === '') add(problems, 'avatar.oscRoot', copy.text.rootRequired)
  if (candidate.plugins.devRoots.some((root) => root.trim() === '')) {
    add(problems, 'plugins.devRoots', copy.text.devRootRequired)
  }
  if (!finiteNumbers(candidate.processing.defaultChannel)) {
    add(problems, 'processing.defaultChannel', copy.text.finiteProcessing)
  }
  if (!Number.isFinite(candidate.processing.activeStaleAfterMs)) {
    add(problems, 'processing.activeStaleAfterMs', copy.text.finiteActiveStale)
  }
  const names = candidate.processing.overrides.map((override) => override.name.trim())
  if (names.some((name) => name === '') || new Set(names).size !== names.length
    || candidate.processing.overrides.some((override) => !finiteNumbers(override.channel))) {
    add(problems, 'processing.overrides', copy.text.invalidOverrides)
  }
  if (candidate.processing.mutualExclusion.some((group) => group.some((name) => name.trim() === ''))) {
    add(problems, 'processing.mutualExclusion', copy.text.groupNameRequired)
  }
  if (candidate.osc.targetMode !== 'auto' && candidate.osc.targetMode !== 'manual') {
    add(problems, 'osc.targetMode', copy.text.invalidMode)
  }
  if (candidate.osc.targetMode === 'manual') {
    if (candidate.osc.preferredService.trim() !== '') {
      add(problems, 'osc.preferredService', copy.text.invalidPreferred)
    }
    if (candidate.osc.manualHost.trim() === '') add(problems, 'osc.manualHost', copy.text.hostRequired)
    if (!Number.isInteger(candidate.osc.manualPort) || candidate.osc.manualPort < 1 || candidate.osc.manualPort > 65535) {
      add(problems, 'osc.manualPort', copy.text.invalidPort)
    }
  }
  return problems
}

function finiteNumbers(value: unknown): boolean {
  if (typeof value === 'number') return Number.isFinite(value)
  if (value === null || typeof value !== 'object') return true
  return Object.values(value).every(finiteNumbers)
}

function add(problems: Map<SettingsField, ProblemView>, field: SettingsField, detail: string) {
  problems.set(field, Object.freeze({
    code: 'validation', title: copy.text.checkInput, detail, tone: 'danger', persistent: false, field,
  }))
}
