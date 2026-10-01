import {fireEvent, render, screen, waitFor, within} from '@testing-library/svelte'
import {tick} from 'svelte'
import {describe, expect, it, vi} from 'vitest'
import OverviewPage from './OverviewPage.svelte'
import {createPluginsFixture, createRuntimeFixture} from './page-test-fixtures.svelte.js'
import type {PluginsModuleState, PluginView} from '../lib/modules/plugins/types.js'
import type {RuntimeModuleState, RuntimeView} from '../lib/modules/runtime/types.js'

function plugin(id: string, overrides: Partial<PluginView> = {}): PluginView {
  return {id, name: id, description: '', version: '1', capabilities: ['eye', 'expression', 'lip'], enabled: true, active: true, state: 'running', configRevision: 1, frameRate: 60, consecutiveFailures: 0, restartCount: 0, installedAt: '2026-10-01T00:00:00Z', ...overrides}
}
function runtimeView(overrides: Partial<RuntimeView> = {}): RuntimeView {
  return {phase: 'running', platformSupported: true, avatar: {id: 'avtr_demo', name: 'Demo Avatar'}, plan: {generation: 8, status: 'ready', source: 'avatar_config', configPath: 'C:/Avatar/demo.json', configId: 'avtr_demo', generationExhausted: false, parameters: [{name: 'v2/EyeLeftX', driven: true}, {name: 'v2/MouthSmileRight', driven: false}]}, osc: {state: 'discovered', target: {host: '127.0.0.1', port: 9000}}, pluginFailures: [], ...overrides}
}
function runtimeState(overrides: Partial<RuntimeModuleState> = {}): RuntimeModuleState {
  return {status: 'ready', snapshot: runtimeView(), revision: 8, updatedAt: '2026-10-02T00:00:00Z', problem: null, ...overrides}
}
function pluginsState(items: PluginView[] = [plugin('eye')]): PluginsModuleState {
  return {status: 'ready', snapshot: {plugins: items}, revision: 3, updatedAt: '2026-10-02T00:00:00Z', problem: null, query: {query: '', filter: 'all', page: 1, pageSize: 24}, visiblePlugins: items, pageCount: 1, filteredTotal: items.length, summary: {total: items.length, enabled: items.length, active: items.length, problem: 0}, pendingIds: new Set(), problems: new Map(), commands: {pending: new Set(), problems: new Map()}}
}
function mount(initial = runtimeState(), items = [plugin('eye')]) {
  const runtime = createRuntimeFixture(initial)
  const setEnabled = vi.fn(async (_id: string, _enabled: boolean) => {})
  const plugins = createPluginsFixture(pluginsState(items), {setQuery: () => {}, setFilter: () => {}, setPage: () => {}, setEnabled})
  const onNavigate = vi.fn()
  render(OverviewPage, {props: {runtime: runtime.module, plugins: plugins.module, onNavigate}})
  return {runtime, plugins, setEnabled, onNavigate}
}
describe('OverviewPage', () => {
  it('shows ready drive counts and parameter flags', async () => {
    mount()
    expect(screen.getByText('当前驱动 1 / 可驱动 2')).toBeVisible()
    const trigger = screen.getByRole('button', {name: '查看参数列表'})
    trigger.focus()
    await fireEvent.click(trigger)
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByText('v2/EyeLeftX')).toBeVisible()
    expect(within(dialog).getByText('当前驱动')).toBeVisible()
    expect(within(dialog).getByText('暂无有效输入')).toBeVisible()
    await fireEvent.keyDown(dialog, {key: 'Escape'})
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await waitFor(() => expect(trigger).toHaveFocus())
  })
  it('replaces avatar content for empty pending and failed plans', async () => {
    const {runtime, onNavigate} = mount(runtimeState({snapshot: runtimeView({plan: {...runtimeView().plan!, parameters: []}})}))
    expect(screen.getByText(/没有请求软件支持的驱动参数/)).toBeVisible()
    runtime.setState(runtimeState({snapshot: runtimeView({avatar: {id: '', name: ''}, plan: undefined})}))
    await tick()
    expect(screen.getByText(/等待 Avatar/)).toBeVisible()
    runtime.setState(runtimeState({snapshot: runtimeView({plan: {...runtimeView().plan!, status: 'pending', parameters: []}})}))
    await tick()
    expect(screen.getByText(/正在准备计划/)).toBeVisible()
    runtime.setState(runtimeState({snapshot: runtimeView({plan: {...runtimeView().plan!, status: 'failed', configPath: '', parameters: []}, planError: 'configuration not found'})}))
    await tick()
    expect(screen.getByText(/未找到当前 Avatar 的配置/)).toBeVisible()
    await fireEvent.click(screen.getAllByRole('button', {name: '前往设置'})[0]!)
    expect(onNavigate).toHaveBeenCalledWith('settings')
  })
  it('shows three capable plugins with quick switches', async () => {
    const items = [plugin('working'), plugin('fault', {state: 'crashed'}), plugin('recovery', {state: 'backoff'}), plugin('other', {state: 'disabled'})]
    const {setEnabled, onNavigate} = mount(runtimeState(), items)
    const rows = screen.getAllByTestId('overview-plugin-row')
    expect(rows.map((row) => row.getAttribute('data-plugin-id'))).toEqual(['fault', 'recovery', 'working'])
    expect(within(rows[2]!).getByLabelText('Eye 输出能力')).toBeVisible()
    expect(within(rows[2]!).getByLabelText('Expression 输出能力')).toBeVisible()
    expect(within(rows[2]!).getByLabelText('Lip 输出能力')).toBeVisible()
    await fireEvent.click(within(rows[2]!).getByRole('button', {name: '停用 working'}))
    expect(setEnabled).toHaveBeenCalledWith('working', false)
    await fireEvent.click(screen.getByRole('button', {name: '查看全部插件'}))
    expect(onNavigate).toHaveBeenCalledWith('plugins')
  })
  it('keeps drive count during OSC failure', () => {
    mount(runtimeState({snapshot: runtimeView({osc: {state: 'error', error: 'OSC offline'}})}))
    expect(screen.getByText('当前驱动 1 / 可驱动 2')).toBeVisible()
    expect(screen.getByText('OSC offline')).toBeVisible()
  })
  it('marks retained drive data as stale', () => {
    mount(runtimeState({status: 'stale'}))
    expect(screen.getByText('最近记录 1 / 可驱动 2')).toBeVisible()
    expect(screen.queryByText('当前驱动 1 / 可驱动 2')).not.toBeInTheDocument()
  })
})
