import {fireEvent, render, screen, waitFor} from '@testing-library/svelte'
import {describe, expect, it, vi} from 'vitest'

import DiagnosticsPage from './DiagnosticsPage.svelte'
import {createPluginsFixture, createRuntimeFixture} from './page-test-fixtures.svelte.js'
import type {PluginsModule, PluginsModuleState} from '../lib/modules/plugins/types.js'
import type {RuntimeModuleState, RuntimeView} from '../lib/modules/runtime/types.js'
import type {SettingsModule, SettingsModuleState} from '../lib/modules/settings/types.js'
import type {ProblemView} from '../lib/presentation/problem.js'

const problem: ProblemView = {
  code: 'unavailable', title: '当前功能暂不可用', detail: '服务暂时不可用。', tone: 'warning', persistent: true,
}

function runtimeView(overrides: Partial<RuntimeView> = {}): RuntimeView {
  return {
    phase: 'running', platformSupported: true, lifecycle: 'started',
    avatar: {name: 'Demo Avatar', id: 'avtr_demo'},
    plan: {
      status: 'ready', source: 'VRChat', generation: 8,
      configId: 'avtr_demo', configPath: 'C:/Users/name/AppData/LocalLow/VRChat/VRChat/OSC/avatar.json', generationExhausted: false,
    },
    osc: {state: 'manual', target: {host: '127.0.0.1', port: 9000}, error: '目标暂时不可达'},
    pluginFailures: [{pluginId: 'eye', operation: 'start', message: 'Eye Tracker 启动失败'}],
    planError: '计划不可用', runtimeError: '运行时提示',
    ...overrides,
  }
}

function runtimeState(overrides: Partial<RuntimeModuleState> = {}): RuntimeModuleState {
  return {status: 'stale', snapshot: runtimeView(), revision: 8, updatedAt: '2026-09-01T10:00:00Z', problem, ...overrides}
}

function pluginsState(overrides: Partial<PluginsModuleState> = {}): PluginsModuleState {
  return {
    status: 'ready', snapshot: {plugins: []}, revision: 3, updatedAt: '2026-09-01T10:00:01Z', problem: null,
    query: {query: '', filter: 'all', page: 1, pageSize: 24}, visiblePlugins: [], pageCount: 1, filteredTotal: 0,
    summary: {total: 0, enabled: 0, active: 0, problem: 0}, pendingIds: new Set(), problems: new Map(),
    commands: {pending: new Set(), problems: new Map()}, ...overrides,
  }
}

function settingsState(overrides: Partial<SettingsModuleState> = {}): SettingsModuleState {
  return {
    status: 'problem', server: null, draft: null, revision: null, fileRevision: null, updatedAt: null, problem,
    fieldProblems: new Map(), dirty: false, validating: false, saving: false, restartRequired: false, conflict: false,
    ...overrides,
  }
}

function settingsFixture(state = settingsState()): SettingsModule {
  return {
    state,
    start: async () => {}, refresh: async () => {}, dispose: () => {}, updateDraft: () => {},
    validate: async () => false, save: async () => false, reload: async () => false,
  }
}

function renderDiagnostics(options: {
  runtime?: RuntimeModuleState
  plugins?: PluginsModuleState
  settings?: SettingsModuleState
} = {}) {
  const runtime = createRuntimeFixture(options.runtime ?? runtimeState())
  const plugins = createPluginsFixture(options.plugins ?? pluginsState(), {
    setQuery: () => {}, setFilter: () => {}, setPage: () => {}, setEnabled: async () => {},
  })
  render(DiagnosticsPage, {props: {runtime: runtime.module, plugins: plugins.module, settings: settingsFixture(options.settings)}})
}

describe('DiagnosticsPage', () => {
  it('presents independent module health and bounded runtime diagnostics without private data', () => {
    renderDiagnostics()

    expect(screen.getByRole('heading', {name: 'Project Status'})).toBeVisible()
    expect(screen.getByText('Runtime')).toBeVisible()
    expect(screen.getByText('Plugins')).toBeVisible()
    expect(screen.getByText('Settings')).toBeVisible()
    expect(screen.getByText('数据可能已过期')).toBeVisible()
    expect(screen.getByText('修订 8')).toBeVisible()
    expect(screen.getByText('2026-09-01T10:00:00Z')).toBeVisible()
    expect(screen.getByText('started')).toBeVisible()
    expect(screen.getByText('受支持')).toBeVisible()
    expect(screen.getByText('VRChat')).toBeVisible()
    expect(screen.getByText('avtr_demo')).toBeVisible()
    expect(screen.getByText('127.0.0.1:9000')).toBeVisible()
    expect(screen.getByText('Eye Tracker 启动失败')).toBeVisible()
    expect(screen.getAllByText('当前功能暂不可用').length).toBeGreaterThan(0)
    expect(screen.getByText('没有已发现的插件')).toBeVisible()
    expect(screen.queryByText(/sessionId|executablePath|pluginConfig|C:\/Users\/name\/AppData/i)).not.toBeInTheDocument()
  })

  it('keeps loading, no-data, unsupported, and startup-failure states actionable per module', () => {
    renderDiagnostics({
      runtime: runtimeState({snapshot: runtimeView({platformSupported: false})}),
      plugins: pluginsState({status: 'problem', snapshot: null, revision: null, updatedAt: null, problem}),
      settings: settingsState({status: 'ready', problem: null}),
    })

    expect(screen.getByText('当前平台暂不受支持')).toBeVisible()
    expect(screen.getByText('Plugins 启动失败')).toBeVisible()
    expect(screen.getByText('Settings 尚无数据')).toBeVisible()
  })

  it('shows a generic Avatar plan-error status without exposing or copying its internal detail', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, {clipboard: {writeText}})
    renderDiagnostics({runtime: runtimeState({snapshot: runtimeView({planError: 'internal plan failure: C:/secret.json'})})})

    expect(screen.getByText('Avatar 计划需要处理')).toBeVisible()
    expect(screen.queryByText(/internal plan failure|secret\.json/i)).not.toBeInTheDocument()
    await fireEvent.click(screen.getByRole('button', {name: '复制诊断信息'}))
    await waitFor(() => expect(writeText).toHaveBeenCalledOnce())
    expect(writeText.mock.calls[0]?.[0]).not.toMatch(/internal plan failure|secret\.json/i)
  })

  it('copies only a bounded, explicitly constructed safe diagnostics summary', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, {clipboard: {writeText}})
    renderDiagnostics()

    await fireEvent.click(screen.getByRole('button', {name: '复制诊断信息'}))

    await waitFor(() => expect(writeText).toHaveBeenCalledOnce())
    const copied = writeText.mock.calls[0]?.[0] as string
    expect(copied).toContain('Runtime: stale')
    expect(copied).toContain('Plugins: ready')
    expect(copied).toContain('Settings: problem')
    expect(copied).toContain('OSC: manual 127.0.0.1:9000')
    expect(copied).not.toMatch(/sessionId|executablePath|pluginConfig|C:\/Users\/name\/AppData|计划不可用|运行时提示/i)
    expect(copied.length).toBeLessThanOrEqual(2048)
  })

  it('catches clipboard rejection without rendering a copied raw diagnostic', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('clipboard unavailable: sessionId=private'))
    Object.assign(navigator, {clipboard: {writeText}})
    renderDiagnostics()

    await fireEvent.click(screen.getByRole('button', {name: '复制诊断信息'}))
    await waitFor(() => expect(writeText).toHaveBeenCalledOnce())
    await Promise.resolve()
    expect(screen.queryByText(/clipboard unavailable|sessionId=private/i)).not.toBeInTheDocument()
  })
})
