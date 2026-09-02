import {fireEvent, render, screen, within} from '@testing-library/svelte'
import {describe, expect, it, vi} from 'vitest'

import OverviewPage from './OverviewPage.svelte'
import type {PluginsModule} from '../lib/modules/plugins/types.js'
import type {RuntimeModule, RuntimeModuleState} from '../lib/modules/runtime/types.js'
import type {ProblemView} from '../lib/presentation/problem.js'

const unavailableProblem: ProblemView = {
  code: 'unavailable',
  title: '当前功能暂不可用',
  detail: '请稍后重试。',
  tone: 'warning',
  persistent: true,
}

function runtimeState(overrides: Partial<RuntimeModuleState> = {}): RuntimeModuleState {
  return {
    status: 'ready',
    snapshot: {
      phase: 'running',
      platformSupported: true,
      avatar: {name: 'Demo Avatar', id: 'avtr_demo'},
      osc: {state: 'discovered', target: {host: '127.0.0.1', port: 9000}},
      plan: {
        status: 'ready',
        source: 'VRChat',
        generation: 8,
        configPath: 'C:/Avatar/demo.json',
        configId: 'avtr_demo',
        generationExhausted: false,
      },
      pluginFailures: [{pluginId: 'eye', operation: 'start', message: 'Eye Tracker 启动失败'}],
    },
    revision: 8,
    updatedAt: '2026-09-01T10:00:00Z',
    problem: null,
    ...overrides,
  }
}

function runtime(state = runtimeState()): RuntimeModule {
  return {
    state,
    start: async () => {},
    refresh: async () => {},
    dispose: () => {},
  }
}

function plugins(): PluginsModule {
  return {
    state: {
      status: 'ready',
      snapshot: {plugins: []},
      revision: 3,
      updatedAt: '2026-09-01T10:00:00Z',
      problem: null,
      query: {query: '', filter: 'all', page: 1, pageSize: 24},
      visiblePlugins: [],
      pageCount: 1,
      filteredTotal: 3,
      summary: {total: 3, enabled: 2, active: 2, problem: 1},
      pendingIds: new Set(),
      problems: new Map(),
      commands: {pending: new Set(), problems: new Map()},
    },
    start: async () => {},
    refresh: async () => {},
    dispose: () => {},
    setQuery: () => {},
    setFilter: () => {},
    setPage: () => {},
    setEnabled: async () => {},
  }
}

describe('OverviewPage', () => {
  it('renders authoritative avatar, OSC output, plan and plugin facts without discovery ports', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, {clipboard: {writeText}})

    render(OverviewPage, {props: {runtime: runtime(), plugins: plugins()}})

    expect(screen.getByRole('main', {name: '概览'})).toBeVisible()
    expect(screen.getByRole('heading', {name: '概览'})).toBeVisible()
    expect(screen.getByText('Demo Avatar')).toBeVisible()
    expect(within(screen.getByRole('region', {name: '当前 Avatar'})).getByText('avtr_demo')).toBeVisible()
    expect(screen.getByText('自动发现')).toBeVisible()
    expect(screen.getByText('127.0.0.1:9000')).toBeVisible()
    expect(screen.getByText('VRChat')).toBeVisible()
    expect(screen.getByText('8')).toBeVisible()
    expect(screen.getByText(/3 个插件/)).toBeVisible()
    expect(screen.getByText('Eye Tracker 启动失败')).toBeVisible()
    expect(screen.queryByText(/监听端口/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/OSCQuery/i)).not.toBeInTheDocument()

    await fireEvent.click(screen.getByRole('button', {name: '复制 Avatar ID'}))
    expect(writeText).toHaveBeenCalledWith('avtr_demo')
  })

  it('distinguishes initial loading, no data, stale data and runtime problems', () => {
    const {rerender} = render(OverviewPage, {
      props: {runtime: runtime(runtimeState({status: 'loading', snapshot: null, problem: null})), plugins: plugins()},
    })
    expect(screen.getByRole('status')).toHaveTextContent('正在读取运行状态')

    rerender({runtime: runtime(runtimeState({status: 'problem', snapshot: null, problem: unavailableProblem})), plugins: plugins()})
    expect(screen.getByText('当前功能暂不可用')).toBeVisible()
    expect(screen.getByText('暂无可显示的运行状态')).toBeVisible()

    rerender({runtime: runtime(runtimeState({status: 'stale', problem: unavailableProblem})), plugins: plugins()})
    expect(screen.getByText('数据可能已过期')).toBeVisible()
    expect(screen.getByText('Demo Avatar')).toBeVisible()

    rerender({runtime: runtime(runtimeState({status: 'problem', problem: unavailableProblem})), plugins: plugins()})
    expect(screen.getByText('当前功能暂不可用')).toBeVisible()
    expect(screen.getByText('Demo Avatar')).toBeVisible()
  })
})
