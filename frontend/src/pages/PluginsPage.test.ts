import {fireEvent, render, screen} from '@testing-library/svelte'
import {tick} from 'svelte'
import {describe, expect, it, vi} from 'vitest'

import PluginsPage from './PluginsPage.svelte'
import {createPluginsFixture} from './page-test-fixtures.svelte.js'
import type {PluginFilter, PluginsModuleState, PluginView} from '../lib/modules/plugins/types.js'
import type {ProblemView} from '../lib/presentation/problem.js'

const unavailableProblem: ProblemView = {
  code: 'unavailable', title: '当前功能暂不可用', detail: 'Eye Tracker 正在重启。', tone: 'warning', persistent: true,
}

function plugin(id: string, overrides: Partial<PluginView> = {}): PluginView {
  return {
    id, name: id === 'eye' ? 'Eye Tracker' : `Plugin ${id}`, description: `${id} description`, version: '1.0.0',
    capabilities: ['Eye', 'Expression'], enabled: true, active: true, state: 'running', configRevision: 1,
    frameRate: 60, consecutiveFailures: 0, restartCount: 0, ...overrides,
  }
}

function state(overrides: Partial<PluginsModuleState> = {}): PluginsModuleState {
  const items = Array.from({length: 25}, (_, index) => plugin(`plugin-${index + 1}`))
  return {
    status: 'ready', snapshot: {plugins: items}, revision: 1, updatedAt: '2026-09-01T10:00:00Z', problem: null,
    query: {query: '', filter: 'all', page: 1, pageSize: 24}, visiblePlugins: items.slice(0, 24), pageCount: 2,
    filteredTotal: 25, summary: {total: 25, enabled: 25, active: 25, problem: 0}, pendingIds: new Set(), problems: new Map(),
    commands: {pending: new Set(), problems: new Map()}, ...overrides,
  }
}

function fixture(initial = state()) {
  const commands = {
    setQuery: vi.fn<(query: string) => void>(),
    setFilter: vi.fn<(filter: PluginFilter) => void>(),
    setPage: vi.fn<(page: number) => void>(),
    setEnabled: vi.fn<(pluginId: string, enabled: boolean) => Promise<void>>(async () => {}),
  }
  return {...createPluginsFixture(initial, commands), commands}
}

describe('PluginsPage', () => {
  it('forwards search, changes only new filters, and honours both pagination ends', async () => {
    const injected = fixture()
    render(PluginsPage, {props: {plugins: injected.module}})

    expect(screen.getByRole('main', {name: '插件'})).toHaveClass('min-w-0')
    expect(screen.getAllByRole('article')).toHaveLength(24)
    expect(screen.getByRole('button', {name: '上一页'})).toBeDisabled()
    expect(screen.getByRole('button', {name: '下一页'})).toBeEnabled()
    await fireEvent.click(screen.getByRole('button', {name: '全部'}))
    await fireEvent.input(screen.getByRole('searchbox', {name: '搜索插件'}), {target: {value: 'eye'}})
    await fireEvent.click(screen.getByRole('button', {name: '只看问题'}))
    await fireEvent.click(screen.getByRole('button', {name: '下一页'}))
    expect(injected.commands.setFilter).toHaveBeenCalledTimes(1)
    expect(injected.commands.setFilter).toHaveBeenCalledWith('problem')
    expect(injected.commands.setQuery).toHaveBeenLastCalledWith('eye')
    expect(injected.commands.setPage).toHaveBeenCalledWith(2)

    injected.setState(state({query: {query: 'eye', filter: 'problem', page: 2, pageSize: 24}, visiblePlugins: [plugin('plugin-25')]}))
    await tick()
    expect(screen.getByRole('button', {name: '上一页'})).toBeEnabled()
    expect(screen.getByRole('button', {name: '下一页'})).toBeDisabled()
    await fireEvent.click(screen.getByRole('button', {name: '上一页'}))
    expect(injected.commands.setPage).toHaveBeenLastCalledWith(1)
  })

  it('prevents repeat intent for the pending ID while another plugin remains actionable', async () => {
    const eye = plugin('eye')
    const lip = plugin('lip', {name: 'Lip Tracker'})
    const injected = fixture(state({
      snapshot: {plugins: [eye, lip]}, visiblePlugins: [eye, lip], pageCount: 1, filteredTotal: 2,
      summary: {total: 2, enabled: 2, active: 2, problem: 0}, pendingIds: new Set(['eye']),
      problems: new Map([['eye', unavailableProblem]]), commands: {pending: new Set(['eye']), problems: new Map([['eye', unavailableProblem]])},
    }))
    render(PluginsPage, {props: {plugins: injected.module}})

    expect(screen.getByRole('switch', {name: '启用 Eye Tracker'})).toBeDisabled()
    expect(screen.getByRole('switch', {name: '启用 Lip Tracker'})).toBeEnabled()
    expect(screen.getByText('Eye Tracker 正在重启。')).toBeVisible()
    await fireEvent.click(screen.getByRole('switch', {name: '启用 Eye Tracker'}))
    await fireEvent.click(screen.getByRole('switch', {name: '启用 Lip Tracker'}))
    expect(injected.commands.setEnabled).toHaveBeenCalledTimes(1)
    expect(injected.commands.setEnabled).toHaveBeenCalledWith('lip', false)
  })

  it('renders local command Problems and bounded last-error fallback per card', () => {
    const eye = plugin('eye')
    const lip = plugin('lip', {name: 'Lip Tracker', lastError: 'Lip tracker lost heartbeat'})
    const injected = fixture(state({
      snapshot: {plugins: [eye, lip]}, visiblePlugins: [eye, lip], pageCount: 1, filteredTotal: 2,
      summary: {total: 2, enabled: 2, active: 2, problem: 1}, problems: new Map([['eye', unavailableProblem]]),
      commands: {pending: new Set(), problems: new Map([['eye', unavailableProblem]])},
    }))
    render(PluginsPage, {props: {plugins: injected.module}})

    expect(screen.getByText('Eye Tracker 正在重启。')).toBeVisible()
    expect(screen.getByText('插件运行提示')).toBeVisible()
    expect(screen.getByText('Lip tracker lost heartbeat')).toBeVisible()
  })

  it('reacts to loading, valid-empty, no-data problem, and stale retained snapshot views', async () => {
    const injected = fixture(state({status: 'loading', snapshot: null, visiblePlugins: [], filteredTotal: 0, summary: {total: 0, enabled: 0, active: 0, problem: 0}}))
    render(PluginsPage, {props: {plugins: injected.module}})
    expect(screen.getByRole('status')).toHaveTextContent('正在读取插件列表')

    injected.setState(state({snapshot: {plugins: []}, visiblePlugins: [], pageCount: 1, filteredTotal: 0, summary: {total: 0, enabled: 0, active: 0, problem: 0}}))
    await tick()
    expect(screen.getByText('没有符合条件的插件')).toBeVisible()

    injected.setState(state({status: 'problem', snapshot: null, visiblePlugins: [], filteredTotal: 0, summary: {total: 0, enabled: 0, active: 0, problem: 0}, problem: unavailableProblem}))
    await tick()
    expect(screen.getByText('当前功能暂不可用')).toBeVisible()
    expect(screen.getByText('暂无可显示的插件')).toBeVisible()

    const retained = plugin('eye')
    injected.setState(state({status: 'stale', snapshot: {plugins: [retained]}, visiblePlugins: [retained], pageCount: 1, filteredTotal: 1, summary: {total: 1, enabled: 1, active: 1, problem: 0}, problem: unavailableProblem}))
    await tick()
    expect(screen.getByText('数据可能已过期')).toBeVisible()
    expect(screen.getByText('Eye Tracker')).toBeVisible()
    expect(screen.getByText(/共 1 个插件/)).toBeVisible()
  })

  it('retains plugin cards and summary for a current Plugins Problem', async () => {
    const retained = plugin('eye')
    const injected = fixture(state({
      snapshot: {plugins: [retained]}, visiblePlugins: [retained], pageCount: 1, filteredTotal: 1,
      summary: {total: 1, enabled: 1, active: 1, problem: 0},
    }))
    render(PluginsPage, {props: {plugins: injected.module}})

    injected.setState(state({
      status: 'problem', snapshot: {plugins: [retained]}, visiblePlugins: [retained], pageCount: 1, filteredTotal: 1,
      summary: {total: 1, enabled: 1, active: 1, problem: 0}, problem: unavailableProblem,
    }))
    await tick()

    expect(screen.getByText('当前功能暂不可用')).toBeVisible()
    expect(screen.getByText('Eye Tracker')).toBeVisible()
    expect(screen.getByText(/共 1 个插件/)).toBeVisible()
  })
})
