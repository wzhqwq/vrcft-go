import {copy} from '../../copy/zh-CN.js'

type Tone = 'neutral' | 'success' | 'warning' | 'danger'
export function phasePresentation(phase: string): {label: string; tone: Tone} {
  const label = localizedState(copy.state.phase, phase)
  const tone = phase === 'running' ? 'success'
    : phase === 'failed' ? 'danger'
      : ['degraded', 'unsupported', 'diagnostic'].includes(phase) ? 'warning' : 'neutral'
  return {label, tone}
}

export function localizedState(labels: Record<string, string>, state: string): string {
  return Object.hasOwn(labels, state) ? labels[state]! : copy.state.unknown
}
