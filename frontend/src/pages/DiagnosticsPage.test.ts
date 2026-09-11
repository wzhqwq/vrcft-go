import {fireEvent, render, screen, waitFor} from '@testing-library/svelte'
import {describe, expect, it, vi} from 'vitest'

import DiagnosticsPage from './DiagnosticsPage.svelte'
import {createPluginsFixture, createRuntimeFixture} from './page-test-fixtures.svelte.js'
import type {PluginsModule, PluginsModuleState} from '../lib/modules/plugins/types.js'
import type {RuntimeModuleState, RuntimeView} from '../lib/modules/runtime/types.js'
import type {SettingsModule, SettingsModuleState} from '../lib/modules/settings/types.js'
import type {ProblemView} from '../lib/presentation/problem.js'
import {localTime} from '../lib/presentation/time.js'
import type {RuntimeModule} from '../lib/modules/runtime/types.js'

const problem: ProblemView = {
  code: 'unavailable', title: '当前功能暂不可用', detail: '服务暂时不可用。', tone: 'warning', persistent: true,
}

function runtimeView(overrides: Partial<RuntimeView> = {}): RuntimeView {
  return {
    phase: 'running', platformSupported: true, lifecycle: 'running',
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
    status: 'problem', server: null, draft: null, revision: null, fileRevision: null, updatedAt: null, problem, canSave: false,
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
  diagnostics?: RuntimeModule['diagnostics']
  refreshDiagnostics?: () => Promise<void>
} = {}) {
  const runtime = createRuntimeFixture(options.runtime ?? runtimeState())
  const plugins = createPluginsFixture(options.plugins ?? pluginsState(), {
    setQuery: () => {}, setFilter: () => {}, setPage: () => {}, setEnabled: async () => {},
  })
  return render(DiagnosticsPage, {props: {runtime: {...runtime.module, diagnostics: options.diagnostics, refreshDiagnostics: options.refreshDiagnostics}, plugins: plugins.module, settings: settingsFixture(options.settings)}})
}

describe('DiagnosticsPage', () => {
  it('presents independent module health and bounded runtime diagnostics without private data', () => {
    renderDiagnostics()

    expect(screen.getByRole('heading', {name: '诊断'})).toBeVisible()
    expect(screen.getByText('Runtime')).toBeVisible()
    expect(screen.getByText('Plugins')).toBeVisible()
    expect(screen.getByText('Settings')).toBeVisible()
    expect(screen.getByText('数据可能已过期')).toBeVisible()
    expect(screen.getByText('修订 8')).toBeVisible()
    expect(screen.getByText(localTime('2026-09-01T10:00:00Z'))).toBeVisible()
    expect(screen.getAllByText('运行中')).toHaveLength(2)
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

  it('shows and copies actionable Avatar plan errors with private paths redacted', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, {clipboard: {writeText}})
    renderDiagnostics({runtime: runtimeState({snapshot: runtimeView({planError: 'internal plan failure: C:/secret.json'})})})

    expect(screen.getByText(/internal plan failure: \[PATH\]/)).toBeVisible()
    expect(screen.queryByText(/secret\.json/i)).not.toBeInTheDocument()
    await fireEvent.click(screen.getByRole('button', {name: '复制诊断信息'}))
    await waitFor(() => expect(writeText).toHaveBeenCalledOnce())
    expect(writeText.mock.calls[0]?.[0]).toContain('internal plan failure: [PATH]')
    expect(writeText.mock.calls[0]?.[0]).not.toMatch(/secret\.json/i)
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
    expect(copied).toContain('unavailable')
    expect(copied).toContain('OSC: manual 127.0.0.1:9000')
    expect(copied).not.toMatch(/sessionId|executablePath|pluginConfig|C:\/Users\/name\/AppData/i)
    expect(copied).toContain('计划不可用')
    expect(copied).toContain('运行时提示')
    expect(copied.length).toBeLessThanOrEqual(16384)
    expect(await screen.findByText('已复制')).toBeVisible()
  })

  it('catches clipboard rejection without rendering a copied raw diagnostic', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('clipboard unavailable: sessionId=private'))
    Object.assign(navigator, {clipboard: {writeText}})
    renderDiagnostics()

    await fireEvent.click(screen.getByRole('button', {name: '复制诊断信息'}))
    await waitFor(() => expect(writeText).toHaveBeenCalledOnce())
    await Promise.resolve()
    expect(await screen.findByText('复制失败，请重试')).toBeVisible()
    expect(screen.queryByText(/clipboard unavailable|sessionId=private/i)).not.toBeInTheDocument()
  })

  it('shows startup details even without Runtime data and filters the copied recent logs', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, {clipboard: {writeText}})
    const failure = {id: 'startup-42', time: '2026-09-10T04:30:00Z', level: 'ERROR', component: 'root', stage: 'backend_start', message: 'listen udp :9001: address already in use token=private'}
    const refreshDiagnostics = vi.fn().mockResolvedValue(undefined)
    const view = renderDiagnostics({
      runtime: runtimeState({snapshot: null, revision: null, updatedAt: null}), refreshDiagnostics,
      diagnostics: {loading: false, error: null, snapshot: {
        failure, logPath: 'C:/Users/alice/AppData/Roaming/vrcft-go/logs', diskError: '',
        entries: [{...failure, id: 'info-1', level: 'INFO', message: 'initializing'}, failure],
      }},
    })
    expect(screen.getAllByText(/listen udp :9001/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/backend_start/).length).toBeGreaterThan(0)
    expect(screen.queryByText(/token=private|Users\/alice/)).not.toBeInTheDocument()
    expect(refreshDiagnostics).toHaveBeenCalledOnce()
    await fireEvent.change(screen.getByRole('combobox', {name: '日志级别'}), {target: {value: 'ERROR'}})
    expect(screen.queryByText('initializing')).not.toBeInTheDocument()
    await fireEvent.click(screen.getByRole('button', {name: '复制日志'}))
    expect(writeText.mock.calls[0]?.[0]).toContain('startup-42')
    expect(writeText.mock.calls[0]?.[0]).not.toMatch(/initializing|token=private/)
    await fireEvent.click(screen.getByRole('button', {name: '刷新日志'}))
    expect(refreshDiagnostics).toHaveBeenCalledTimes(2)
    view.unmount()
  })
  it('polls diagnostics only while the page is mounted', async () => {
    vi.useFakeTimers()
    try {
      const refreshDiagnostics = vi.fn().mockResolvedValue(undefined)
      const view = renderDiagnostics({refreshDiagnostics})
      expect(refreshDiagnostics).toHaveBeenCalledOnce()
      await vi.advanceTimersByTimeAsync(3000)
      expect(refreshDiagnostics).toHaveBeenCalledTimes(2)
      view.unmount()
      await vi.advanceTimersByTimeAsync(6000)
      expect(refreshDiagnostics).toHaveBeenCalledTimes(2)
    } finally { vi.useRealTimers() }
  })
  it('prioritizes the startup failure in a bounded summary with many plugin failures', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, {clipboard: {writeText}})
    renderDiagnostics({
      runtime: runtimeState({snapshot: runtimeView({pluginFailures: Array.from({length: 30}, (_, i) => ({pluginId: String(i), operation: 'start', message: 'x'.repeat(2000)}))})}),
      diagnostics: {loading: false, error: null, snapshot: {entries: [], diskError: '', logPath: '', failure: {id: 'priority-startup', time: '', level: 'ERROR', component: 'root', stage: 'backend_start', message: 'startup detail'}}},
    })
    await fireEvent.click(screen.getByRole('button', {name: '复制诊断信息'}))
    expect(writeText.mock.calls[0]?.[0]).toContain('priority-startup')
    expect((writeText.mock.calls[0]?.[0] as string).length).toBeLessThanOrEqual(16384)
  })
})
