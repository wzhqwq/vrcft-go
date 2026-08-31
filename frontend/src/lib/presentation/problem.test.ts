import {describe, expect, it} from 'vitest'

import type {ProblemWire} from '../wails/types'
import {presentProblem} from './problem'

describe('presentProblem', () => {
  it.each([
    [
      'validation',
      {code: 'validation', message: '请检查输入', field: 'settings.avatar.oscRoot'} satisfies ProblemWire,
      {
        tone: 'danger',
        title: '输入内容需要调整',
        detail: '请检查输入',
        persistent: false,
      },
    ],
    [
      'conflict',
      {code: 'conflict', message: '服务器已更新', currentRevision: 8} satisfies ProblemWire,
      {
        tone: 'warning',
        title: '数据已在其他位置更新',
        detail: '当前版本已更新到修订 8，请重新加载后再试。',
        persistent: true,
      },
    ],
    [
      'not_found',
      {code: 'not_found', message: '未找到目标'} satisfies ProblemWire,
      {
        tone: 'warning',
        title: '未找到所需内容',
        detail: '未找到目标',
        persistent: false,
      },
    ],
    [
      'unavailable',
      {code: 'unavailable', message: '服务暂不可用'} satisfies ProblemWire,
      {
        tone: 'warning',
        title: '当前功能暂不可用',
        detail: '服务暂不可用',
        persistent: true,
      },
    ],
    [
      'unsupported_platform',
      {code: 'unsupported_platform', message: '仅支持 Windows'} satisfies ProblemWire,
      {
        tone: 'warning',
        title: '当前平台暂不受支持',
        detail: '仅支持 Windows',
        persistent: true,
      },
    ],
    [
      'timeout',
      {code: 'timeout', message: '请求超时'} satisfies ProblemWire,
      {
        tone: 'warning',
        title: '操作响应超时',
        detail: '请求超时',
        persistent: false,
      },
    ],
    [
      'internal',
      {code: 'internal', message: 'safe'} satisfies ProblemWire,
      {
        tone: 'danger',
        title: '应用发生内部错误',
        detail: 'safe',
        persistent: true,
      },
    ],
    [
      'unknown fallback',
      {code: 'future_code', message: '未来问题'} satisfies ProblemWire,
      {
        tone: 'danger',
        title: '发生未知错误',
        detail: '未来问题',
        persistent: true,
      },
    ],
  ])('maps %s problems to stable UI presentation', (_, problem, expected) => {
    expect(presentProblem(problem)).toMatchObject(expected)
  })
})
