import {fireEvent, render, screen, waitFor, within} from '@testing-library/svelte'
import {describe, expect, it, vi} from 'vitest'

import App from './App.svelte'
import type {WailsPorts} from './lib/wails/ports.js'
import type {PluginListWire, RuntimeWire, SettingsCandidate, SettingsWire} from './lib/wails/types.js'

function runtimeWire(): RuntimeWire {
  return {
    revision: 1, updatedAt: '2026-09-01T00:00:00Z', phase: 'running', platformSupported: true,
    application: {
      lifecycle: 'started', avatarId: 'avtr_demo', avatarName: 'Demo Avatar', planGeneration: 1,
      planStatus: 'ready', planSource: 'VRChat', configPath: '', configId: 'avtr_demo', generationExhausted: false,
      osc: {running: true, connected: true, hasTarget: true, targetMode: 'auto', target: {host: '127.0.0.1', port: 9000}},
      pluginFailures: [],
    },
  }
}

function candidate(): SettingsCandidate {
  const channel = {
    calibration: {enabled: true, neutral: 0, min: -1, max: 1, gain: 1, invert: false},
    tuning: {deadzone: 0, gain: 1, exponent: 1, clampEnabled: true, clampMin: -1, clampMax: 1},
    filter: {mode: 'ema', emaAlpha: 0.5, minCutoff: 1, beta: 0, derivativeCutoff: 1},
    dropout: {holdDurationMs: 10, decayDurationMs: 20, staleAfterMs: 30},
  }
  return {
    avatar: {oscRoot: 'C:\\VRChat\\OSC', fallbackPath: 'C:\\VRChat\\fallback.json'}, plugins: {devRoots: []},
    processing: {defaultChannel: channel, overrides: [], activeStaleAfterMs: 1000, mutualExclusion: []},
    osc: {targetMode: 'auto', preferredService: '', manualHost: '127.0.0.1', manualPort: 9000},
  }
}

function pluginWire(): PluginListWire {
  return {revision: 1, updatedAt: '2026-09-01T00:00:00Z', plugins: []}
}

function settingsWire(): SettingsWire {
  return {revision: 1, fileRevision: 1, updatedAt: '2026-09-01T00:00:00Z', settings: candidate()}
}

function ports(overrides: Partial<WailsPorts> = {}) {
  const stops = {runtime: vi.fn(), plugins: vi.fn(), settings: vi.fn()}
  const mock: WailsPorts = {
    runtime: {getStatus: vi.fn(async () => runtimeWire()), onChanged: vi.fn(() => stops.runtime)},
    plugins: {list: vi.fn(async () => pluginWire()), setEnabled: vi.fn(async () => ({...pluginWire(), pluginId: 'x'})), onChanged: vi.fn(() => stops.plugins)},
    settings: {
      get: vi.fn(async () => settingsWire()), validate: vi.fn(async (value) => ({revision: 1, updatedAt: '2026-09-01T00:00:00Z', settings: value})),
      save: vi.fn(async (_revision, value) => ({revision: 2, fileRevision: 2, updatedAt: '2026-09-01T00:00:00Z', settings: value, restartRequired: false})),
      onChanged: vi.fn(() => stops.settings),
    },
    ...overrides,
  }
  return {mock, stops}
}

async function clickNavigation(name: string) {
  await fireEvent.click(screen.getAllByRole('button', {name})[0]!)
}

describe('App', () => {
  it('starts all independent modules concurrently even when Runtime fails', async () => {
    const {mock} = ports()
    const runtime = mock.runtime.getStatus as ReturnType<typeof vi.fn>
    runtime.mockRejectedValueOnce(new Error('runtime unavailable'))
    render(App, {props: {ports: mock}})

    await waitFor(() => {
      expect(mock.runtime.getStatus).toHaveBeenCalledOnce()
      expect(mock.plugins.list).toHaveBeenCalledOnce()
      expect(mock.settings.get).toHaveBeenCalledOnce()
    })
    expect(mock.runtime.onChanged).toHaveBeenCalledOnce()
    expect(mock.plugins.onChanged).toHaveBeenCalledOnce()
    expect(mock.settings.onChanged).toHaveBeenCalledOnce()
    await waitFor(() => expect(screen.getByText('暂无可显示的运行状态')).toBeVisible())
  })

  it('renders one active page and switches clean navigation without confirmation', async () => {
    const {mock} = ports()
    render(App, {props: {ports: mock}})

    expect(await screen.findByRole('main', {name: '概览'})).toBeVisible()
    expect(screen.queryByRole('main', {name: '插件'})).not.toBeInTheDocument()
    await clickNavigation('插件')
    expect(screen.getByRole('main', {name: '插件'})).toBeVisible()
    expect(screen.queryByRole('main', {name: '概览'})).not.toBeInTheDocument()
  })

  it('keeps a dirty Settings draft on cancel and uses the most recent repeated navigation target on confirm', async () => {
    const {mock} = ports()
    render(App, {props: {ports: mock}})
    await screen.findByRole('main', {name: '概览'})
    await clickNavigation('设置')
    const oscRoot = await screen.findByRole('textbox', {name: 'Avatar OSC 根目录'})
    await fireEvent.input(oscRoot, {target: {value: 'C:\\mine'}})

    await clickNavigation('概览')
    const dialog = screen.getByRole('dialog', {name: '放弃未保存的更改？'})
    await clickNavigation('插件')
    await fireEvent.click(within(dialog).getByRole('button', {name: '取消'}))
    expect(screen.getByRole('main', {name: '设置'})).toBeVisible()
    expect(screen.getByRole('textbox', {name: 'Avatar OSC 根目录'})).toHaveValue('C:\\mine')

    await clickNavigation('概览')
    await clickNavigation('插件')
    await fireEvent.click(within(screen.getByRole('dialog', {name: '放弃未保存的更改？'})).getByRole('button', {name: '放弃更改'}))
    expect(screen.getByRole('main', {name: '插件'})).toBeVisible()
    await clickNavigation('设置')
    expect(await screen.findByRole('textbox', {name: 'Avatar OSC 根目录'})).toHaveValue('C:\\mine')
  })

  it('restores the attempted navigation focus after canceling dirty navigation', async () => {
    const {mock} = ports()
    render(App, {props: {ports: mock}})
    await screen.findByRole('main', {name: '概览'})
    await clickNavigation('设置')
    await fireEvent.input(await screen.findByRole('textbox', {name: 'Avatar OSC 根目录'}), {target: {value: 'C:\\mine'}})
    const overview = screen.getAllByRole('button', {name: '概览'})[0]!
    overview.focus()
    await fireEvent.click(overview)
    await fireEvent.click(within(screen.getByRole('dialog', {name: '放弃未保存的更改？'})).getByRole('button', {name: '取消'}))
    await waitFor(() => expect(overview).toHaveFocus())
    expect(screen.getByRole('main', {name: '设置'})).toBeVisible()
  })

  it('blocks beforeunload only while Settings is dirty and disposes every module subscription once', async () => {
    const {mock, stops} = ports()
    const view = render(App, {props: {ports: mock}})
    await screen.findByRole('main', {name: '概览'})
    const clean = new Event('beforeunload', {cancelable: true})
    window.dispatchEvent(clean)
    expect(clean.defaultPrevented).toBe(false)

    await clickNavigation('设置')
    await fireEvent.input(await screen.findByRole('textbox', {name: 'Avatar OSC 根目录'}), {target: {value: 'C:\\mine'}})
    const dirty = new Event('beforeunload', {cancelable: true})
    window.dispatchEvent(dirty)
    expect(dirty.defaultPrevented).toBe(true)

    view.unmount()
    expect(stops.runtime).toHaveBeenCalledOnce()
    expect(stops.plugins).toHaveBeenCalledOnce()
    expect(stops.settings).toHaveBeenCalledOnce()
    const afterUnmount = new Event('beforeunload', {cancelable: true})
    window.dispatchEvent(afterUnmount)
    expect(afterUnmount.defaultPrevented).toBe(false)
  })
})
