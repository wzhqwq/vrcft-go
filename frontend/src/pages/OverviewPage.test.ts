import {fireEvent, render, screen, within} from '@testing-library/svelte'
import {tick} from 'svelte'
import {describe, expect, it, vi} from 'vitest'

import OverviewPage from './OverviewPage.svelte'
import {createPluginsFixture, createRuntimeFixture} from './page-test-fixtures.svelte.js'
import type {PluginsModuleState, PluginView} from '../lib/modules/plugins/types.js'
import type {RuntimeModuleState, RuntimeView} from '../lib/modules/runtime/types.js'
import type {ProblemView} from '../lib/presentation/problem.js'

const unavailableProblem: ProblemView = {
  code: 'unavailable', title: '当前功能暂不可用', detail: '请稍后重试。', tone: 'warning', persistent: true,
}

function plugin(id: string): PluginView {
  return {
    id, name: id, description: '', version: '1.0.0', capabilities: [], enabled: true, active: true,
    state: 'running', configRevision: 1, frameRate: 60, consecutiveFailures: 0, restartCount: 0,
  }
}

function runtimeSnapshot(overrides: Partial<RuntimeView> = {}): RuntimeView {
  return {
    phase: 'running', platformSupported: true, avatar: {name: 'Demo Avatar', id: 'avtr_demo'},
    osc: {state: 'discovered', target: {host: '127.0.0.1', port: 9000}},
    plan: {status: 'ready', source: 'VRChat', generation: 8, configPath: 'C:/Avatar/demo.json', configId: 'avtr_demo', generationExhausted: false},
    pluginFailures: [{pluginId: 'eye', operation: 'start', message: 'Eye Tracker 启动失败'}],
    ...overrides,
  }
}

function runtimeState(overrides: Partial<RuntimeModuleState> = {}): RuntimeModuleState {
  return {status: 'ready', snapshot: runtimeSnapshot(), revision: 8, updatedAt: '2026-09-01T10:00:00Z', problem: null, ...overrides}
}

function pluginsState(overrides: Partial<PluginsModuleState> = {}): PluginsModuleState {
  const items = [plugin('eye'), plugin('lip'), plugin('face')]
  return {
    status: 'ready', snapshot: {plugins: items}, revision: 3, updatedAt: '2026-09-01T10:00:00Z', problem: null,
    query: {query: '', filter: 'all', page: 1, pageSize: 24}, visiblePlugins: items, pageCount: 1, filteredTotal: 3,
    summary: {total: 3, enabled: 2, active: 2, problem: 1}, pendingIds: new Set(), problems: new Map(),
    commands: {pending: new Set(), problems: new Map()}, ...overrides,
  }
}

function pluginsFixture(initial = pluginsState()) {
  return createPluginsFixture(initial, {setQuery: () => {}, setFilter: () => {}, setPage: () => {}, setEnabled: async () => {}})
}

describe('OverviewPage', () => {
  it('renders authoritative runtime phase, avatar, OSC output, plan and plugin failures without discovery ports', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, {clipboard: {writeText}})
    const runtime = createRuntimeFixture(runtimeState())
    const plugins = pluginsFixture()
    render(OverviewPage, {props: {runtime: runtime.module, plugins: plugins.module}})

    expect(screen.getByRole('main', {name: '概览'})).toBeVisible()
    expect(within(screen.getByRole('article', {name: '应用阶段'})).getByText('running')).toBeVisible()
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

  it('reacts to runtime loading, no-data, stale and problem views', async () => {
    const runtime = createRuntimeFixture(runtimeState({status: 'loading', snapshot: null, problem: null}))
    render(OverviewPage, {props: {runtime: runtime.module, plugins: pluginsFixture().module}})
    expect(screen.getByRole('status')).toHaveTextContent('正在读取运行状态')

    runtime.setState(runtimeState({status: 'problem', snapshot: null, problem: unavailableProblem}))
    await tick()
    expect(screen.getByText('当前功能暂不可用')).toBeVisible()
    expect(screen.getByText('暂无可显示的运行状态')).toBeVisible()

    runtime.setState(runtimeState({status: 'stale', problem: unavailableProblem}))
    await tick()
    expect(screen.getByText('数据可能已过期')).toBeVisible()
    expect(screen.getByText('Demo Avatar')).toBeVisible()

    runtime.setState(runtimeState({status: 'problem', problem: unavailableProblem}))
    await tick()
    expect(screen.getByText('当前功能暂不可用')).toBeVisible()
    expect(screen.getByText('Demo Avatar')).toBeVisible()
  })

  it('reacts to independent Plugins loading, no-data, stale and valid-empty views', async () => {
    const runtime = createRuntimeFixture(runtimeState())
    const plugins = pluginsFixture(pluginsState({status: 'loading', snapshot: null, visiblePlugins: [], filteredTotal: 0, summary: {total: 0, enabled: 0, active: 0, problem: 0}}))
    render(OverviewPage, {props: {runtime: runtime.module, plugins: plugins.module}})
    expect(screen.getByRole('status')).toHaveTextContent('正在读取插件概览')
    expect(screen.queryByText(/0 个插件/)).not.toBeInTheDocument()

    plugins.setState(pluginsState({status: 'problem', snapshot: null, visiblePlugins: [], filteredTotal: 0, summary: {total: 0, enabled: 0, active: 0, problem: 0}, problem: unavailableProblem}))
    await tick()
    expect(screen.getByText('当前功能暂不可用')).toBeVisible()
    expect(screen.getByText('暂无可显示的插件概览')).toBeVisible()

    plugins.setState(pluginsState({status: 'stale', problem: unavailableProblem}))
    await tick()
    expect(screen.getByText('数据可能已过期')).toBeVisible()
    expect(screen.getByText(/3 个插件/)).toBeVisible()

    plugins.setState(pluginsState({snapshot: {plugins: []}, visiblePlugins: [], filteredTotal: 0, summary: {total: 0, enabled: 0, active: 0, problem: 0}}))
    await tick()
    expect(screen.getByText('没有已发现的插件')).toBeVisible()
  })

  it('retains the plugins summary and runtime failures for a current Plugins Problem', async () => {
    const runtime = createRuntimeFixture(runtimeState())
    const plugins = pluginsFixture()
    render(OverviewPage, {props: {runtime: runtime.module, plugins: plugins.module}})

    plugins.setState(pluginsState({status: 'problem', problem: unavailableProblem}))
    await tick()

    expect(screen.getByText('当前功能暂不可用')).toBeVisible()
    expect(screen.getByText(/3 个插件/)).toBeVisible()
    expect(screen.getByText('Eye Tracker 启动失败')).toBeVisible()
  })

  it.each([
    ['not_running', '未启动'], ['discovering', '正在发现'], ['discovered', '自动发现'], ['manual', '手动目标'], ['error', '发现出错'],
  ] as const)('renders the %s OSC discovery label', (state, label) => {
    const runtime = createRuntimeFixture(runtimeState({snapshot: runtimeSnapshot({osc: {state, error: state === 'error' ? 'OSC offline' : undefined}})}))
    render(OverviewPage, {props: {runtime: runtime.module, plugins: pluginsFixture().module}})
    expect(screen.getByText(label)).toBeVisible()
    expect(screen.queryByText(/监听端口|OSCQuery/i)).not.toBeInTheDocument()
  })

  it('shows explicit absent OSC and plan facts', () => {
    const runtime = createRuntimeFixture(runtimeState({snapshot: runtimeSnapshot({osc: undefined, plan: undefined})}))
    render(OverviewPage, {props: {runtime: runtime.module, plugins: pluginsFixture().module}})

    expect(screen.getByText('尚未提供 OSC 输出状态。')).toBeVisible()
    expect(screen.getByText('尚未生成可用计划。')).toBeVisible()
  })
})
