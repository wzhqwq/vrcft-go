import {fireEvent, render, screen} from '@testing-library/svelte'
import {describe, expect, it, vi} from 'vitest'

import PluginsPage from './PluginsPage.svelte'
import type {PluginFilter, PluginsModule, PluginsModuleState, PluginView} from '../lib/modules/plugins/types.js'
import type {ProblemView} from '../lib/presentation/problem.js'

const unavailableProblem: ProblemView = {
  code: 'unavailable',
  title: '当前功能暂不可用',
  detail: 'Eye Tracker 正在重启。',
  tone: 'warning',
  persistent: true,
}

function plugin(id: string, overrides: Partial<PluginView> = {}): PluginView {
  return {
    id,
    name: id === 'eye' ? 'Eye Tracker' : `Plugin ${id}`,
    description: `${id} description`,
    version: '1.0.0',
    capabilities: ['Eye', 'Expression'],
    enabled: true,
    active: true,
    state: 'running',
    configRevision: 1,
    frameRate: 60,
    consecutiveFailures: 0,
    restartCount: 0,
    ...overrides,
  }
}

function state(overrides: Partial<PluginsModuleState> = {}): PluginsModuleState {
  const items = Array.from({length: 24}, (_, index) => plugin(`plugin-${index + 1}`))
  return {
    status: 'ready',
    snapshot: {plugins: items},
    revision: 1,
    updatedAt: '2026-09-01T10:00:00Z',
    problem: null,
    query: {query: '', filter: 'all', page: 1, pageSize: 24},
    visiblePlugins: items,
    pageCount: 2,
    filteredTotal: 25,
    summary: {total: 25, enabled: 24, active: 24, problem: 1},
    pendingIds: new Set(),
    problems: new Map(),
    commands: {pending: new Set(), problems: new Map()},
    ...overrides,
  }
}

function module(moduleState = state()) {
  const setQuery = vi.fn<(query: string) => void>()
  const setFilter = vi.fn<(filter: PluginFilter) => void>()
  const setPage = vi.fn<(page: number) => void>()
  const setEnabled = vi.fn<(pluginId: string, enabled: boolean) => Promise<void>>(async () => {})

  return {
    state: moduleState,
    start: async () => {},
    refresh: async () => {},
    dispose: () => {},
    setQuery,
    setFilter,
    setPage,
    setEnabled,
  }
}

describe('PluginsPage', () => {
  it('uses module search, filters and fixed 24-item pagination rather than local selection', async () => {
    const injected = module()
    render(PluginsPage, {props: {plugins: injected}})

    expect(screen.getByRole('main', {name: '插件'})).toHaveClass('min-w-0')
    expect(screen.getAllByRole('article')).toHaveLength(24)
    expect(screen.getByText(/共 25 个插件/)).toBeVisible()
    expect(screen.getByText('第 1 页，共 2 页')).toBeVisible()

    await fireEvent.input(screen.getByRole('searchbox', {name: '搜索插件'}), {target: {value: 'eye'}})
    await fireEvent.click(screen.getByRole('button', {name: '只看问题'}))
    await fireEvent.click(screen.getByRole('button', {name: '下一页'}))

    expect(injected.setQuery).toHaveBeenLastCalledWith('eye')
    expect(injected.setFilter).toHaveBeenCalledWith('problem')
    expect(injected.setPage).toHaveBeenCalledWith(2)
  })

  it('sends one enablement command and isolates pending and problem UI by plugin ID', async () => {
    const eye = plugin('eye')
    const lip = plugin('lip', {name: 'Lip Tracker'})
    const injected = module(state({
      snapshot: {plugins: [eye, lip]},
      visiblePlugins: [eye, lip],
      pageCount: 1,
      filteredTotal: 2,
      summary: {total: 2, enabled: 2, active: 2, problem: 0},
      pendingIds: new Set(['eye']),
      problems: new Map([['eye', unavailableProblem]]),
      commands: {pending: new Set(['eye']), problems: new Map([['eye', unavailableProblem]])},
    }))
    render(PluginsPage, {props: {plugins: injected}})

    expect(screen.getByRole('switch', {name: '启用 Eye Tracker'})).toBeDisabled()
    expect(screen.getByRole('switch', {name: '启用 Lip Tracker'})).toBeEnabled()
    expect(screen.getByText('Eye Tracker 正在重启。')).toBeVisible()

    await fireEvent.click(screen.getByRole('switch', {name: '启用 Lip Tracker'}))
    expect(injected.setEnabled).toHaveBeenCalledTimes(1)
    expect(injected.setEnabled).toHaveBeenCalledWith('lip', false)
  })

  it('shows informative loading, empty and no-data problem states without raw configuration', () => {
    const {rerender} = render(PluginsPage, {props: {plugins: module(state({status: 'loading', snapshot: null, visiblePlugins: []}))}})
    expect(screen.getByRole('status')).toHaveTextContent('正在读取插件列表')

    rerender({plugins: module(state({snapshot: {plugins: []}, visiblePlugins: [], filteredTotal: 0, summary: {total: 0, enabled: 0, active: 0, problem: 0}}))})
    expect(screen.getByText('没有符合条件的插件')).toBeVisible()

    rerender({plugins: module(state({status: 'problem', snapshot: null, visiblePlugins: [], problem: unavailableProblem}))})
    expect(screen.getByText('当前功能暂不可用')).toBeVisible()
    expect(screen.getByText('暂无可显示的插件')).toBeVisible()
    expect(screen.queryByText(/pluginConfig|GetConfig|\{.*\}/)).not.toBeInTheDocument()
  })
})
