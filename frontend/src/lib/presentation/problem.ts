import {copy} from '../../copy/zh-CN'
import type {ProblemWire} from '../wails/types'

export type ProblemTone = 'danger' | 'warning'

export interface ProblemView {
  code: string
  title: string
  detail: string
  tone: ProblemTone
  persistent: boolean
  field?: string
  currentRevision?: number
}

type KnownProblemCode =
  | 'validation'
  | 'conflict'
  | 'not_found'
  | 'unavailable'
  | 'unsupported_platform'
  | 'timeout'
  | 'internal'

interface ProblemPreset {
  title: string
  tone: ProblemTone
  persistent: boolean
  detail(problem: ProblemWire): string
}

const knownProblemPresets = {
  validation: {
    title: copy.problem.validation.title,
    tone: 'danger',
    persistent: false,
    detail: (problem) => problem.message || copy.problem.validation.defaultDetail,
  },
  conflict: {
    title: copy.problem.conflict.title,
    tone: 'warning',
    persistent: true,
    detail: (problem) =>
      typeof problem.currentRevision === 'number'
        ? copy.problem.conflict.detailWithRevision(problem.currentRevision)
        : problem.message || copy.problem.conflict.defaultDetail,
  },
  not_found: {
    title: copy.problem.notFound.title,
    tone: 'warning',
    persistent: false,
    detail: (problem) => problem.message || copy.problem.notFound.defaultDetail,
  },
  unavailable: {
    title: copy.problem.unavailable.title,
    tone: 'warning',
    persistent: true,
    detail: (problem) => problem.message || copy.problem.unavailable.defaultDetail,
  },
  unsupported_platform: {
    title: copy.problem.unsupportedPlatform.title,
    tone: 'warning',
    persistent: true,
    detail: (problem) => problem.message || copy.problem.unsupportedPlatform.defaultDetail,
  },
  timeout: {
    title: copy.problem.timeout.title,
    tone: 'warning',
    persistent: false,
    detail: (problem) => problem.message || copy.problem.timeout.defaultDetail,
  },
  internal: {
    title: copy.problem.internal.title,
    tone: 'danger',
    persistent: true,
    detail: (problem) => problem.message || copy.problem.internal.defaultDetail,
  },
} satisfies Record<KnownProblemCode, ProblemPreset>

const unknownProblemPreset: ProblemPreset = {
  title: copy.problem.unknown.title,
  tone: 'danger',
  persistent: true,
  detail: (problem) => problem.message || copy.problem.unknown.defaultDetail,
}

export function presentProblem(problem: ProblemWire): ProblemView {
  const preset = lookupProblemPreset(problem.code)

  return {
    code: problem.code,
    title: preset.title,
    detail: preset.detail(problem),
    tone: preset.tone,
    persistent: preset.persistent,
    field: problem.field,
    currentRevision: problem.currentRevision,
  }
}

function lookupProblemPreset(code: string): ProblemPreset {
  return code in knownProblemPresets
    ? knownProblemPresets[code as KnownProblemCode]
    : unknownProblemPreset
}
