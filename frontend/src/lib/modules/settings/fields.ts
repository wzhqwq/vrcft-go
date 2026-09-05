import type {ProblemView} from '../../presentation/problem.js'
import type {SettingsCandidate} from '../../wails/types.js'

export const fieldTargets = {
  'avatar.oscRoot': {section: 'general', control: 'avatar-osc-root'},
  'avatar.fallbackPath': {section: 'general', control: 'avatar-fallback-path'},
  'plugins.devRoots': {section: 'general', control: 'plugin-dev-roots'},
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
  if (candidate.avatar.oscRoot.trim() === '') add(problems, 'avatar.oscRoot', 'Avatar OSC 根目录不能为空。')
  if (candidate.plugins.devRoots.some((root) => root.trim() === '')) {
    add(problems, 'plugins.devRoots', '插件开发目录不能为空。')
  }
  if (!finiteNumbers(candidate.processing.defaultChannel)) {
    add(problems, 'processing.defaultChannel', '处理参数必须是有限数字。')
  }
  if (!Number.isFinite(candidate.processing.activeStaleAfterMs)) {
    add(problems, 'processing.activeStaleAfterMs', '活跃通道过期时长必须是有限数字。')
  }
  const names = candidate.processing.overrides.map((override) => override.name.trim())
  if (names.some((name) => name === '') || new Set(names).size !== names.length
    || candidate.processing.overrides.some((override) => !finiteNumbers(override.channel))) {
    add(problems, 'processing.overrides', '覆盖名称必须非空且唯一，处理参数必须是有限数字。')
  }
  if (candidate.processing.mutualExclusion.some((group) => group.some((name) => name.trim() === ''))) {
    add(problems, 'processing.mutualExclusion', '互斥组中的通道名称不能为空。')
  }
  if (candidate.osc.targetMode !== 'auto' && candidate.osc.targetMode !== 'manual') {
    add(problems, 'osc.targetMode', 'OSC 目标模式必须是自动或手动。')
  }
  if (candidate.osc.targetMode === 'manual') {
    if (candidate.osc.preferredService.trim() !== '') {
      add(problems, 'osc.preferredService', '手动模式不能设置首选发现服务。')
    }
    if (candidate.osc.manualHost.trim() === '') add(problems, 'osc.manualHost', '手动模式需要目标主机。')
    if (!Number.isInteger(candidate.osc.manualPort) || candidate.osc.manualPort < 1 || candidate.osc.manualPort > 65535) {
      add(problems, 'osc.manualPort', '端口必须是 1 到 65535 之间的整数。')
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
    code: 'validation', title: '请检查输入', detail, tone: 'danger', persistent: false, field,
  }))
}
