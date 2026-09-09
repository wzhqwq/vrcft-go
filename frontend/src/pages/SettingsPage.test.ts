import {fireEvent, render, screen, waitFor, within} from '@testing-library/svelte'
import {describe, expect, it, vi} from 'vitest'

import SettingsPage from './SettingsPage.svelte'
import {createSettingsModule} from '../lib/modules/settings/index.js'
import type {SettingsPort, Stop} from '../lib/wails/ports.js'
import type {
  ProcessingChannelWire,
  ProblemWire,
  SettingsCandidate,
  SettingsSaveWire,
  SettingsValidationWire,
  SettingsWire,
} from '../lib/wails/types.js'

interface Deferred<T> {
  resolve(value: T): void
}

class SettingsPortFixture implements SettingsPort {
  readonly gets: SettingsWire[] = []
  readonly validations: SettingsCandidate[] = []
  readonly saves: Array<{expectedRevision: number; candidate: SettingsCandidate}> = []
  readonly validationPending: Deferred<SettingsValidationWire>[] = []
  readonly savePending: Deferred<SettingsSaveWire>[] = []

  async get(): Promise<SettingsWire> {
    const next = this.gets.shift()
    if (next === undefined) throw new Error('No queued Settings response')
    return next
  }

  validate(candidate: SettingsCandidate): Promise<SettingsValidationWire> {
    this.validations.push(candidate)
    return new Promise((resolve) => this.validationPending.push({resolve}))
  }

  save(expectedRevision: number, candidate: SettingsCandidate): Promise<SettingsSaveWire> {
    this.saves.push({expectedRevision, candidate})
    return new Promise((resolve) => this.savePending.push({resolve}))
  }

  onChanged(_listener: (value: unknown) => void): Stop {
    return () => {}
  }
}

function channel(): ProcessingChannelWire {
  return {
    calibration: {enabled: true, neutral: 0, min: -1, max: 1, gain: 1, invert: false},
    tuning: {deadzone: 0, gain: 1, exponent: 1, clampEnabled: true, clampMin: -1, clampMax: 1},
    filter: {mode: 'ema', emaAlpha: 0.5, minCutoff: 1, beta: 0, derivativeCutoff: 1},
    dropout: {holdDurationMs: 10, decayDurationMs: 20, staleAfterMs: 30},
  }
}

function candidate(overrides: Partial<SettingsCandidate> = {}): SettingsCandidate {
  return {
    avatar: {oscRoot: 'C:\\VRChat\\OSC', fallbackPath: 'C:\\VRChat\\fallback.json'},
    plugins: {devRoots: ['C:\\plugins']},
    processing: {
      defaultChannel: channel(),
      overrides: [{name: 'eye.left_gaze_x', channel: channel()}],
      activeStaleAfterMs: 1000,
      mutualExclusion: [['eye.left_gaze_x', 'eye.right_gaze_x']],
    },
    osc: {targetMode: 'auto', preferredService: 'VRChat-Client', manualHost: '127.0.0.1', manualPort: 9000},
    ...overrides,
  }
}

function wire(revision: number, settings = candidate(), problem?: ProblemWire): SettingsWire {
  return {revision, updatedAt: '2026-09-01T00:00:00Z', fileRevision: revision, settings, problem}
}

function validation(revision: number, settings: SettingsCandidate, problem?: ProblemWire): SettingsValidationWire {
  return {revision, updatedAt: '2026-09-01T00:00:01Z', settings, problem}
}

function save(revision: number, settings: SettingsCandidate, restartRequired: boolean, problem?: ProblemWire): SettingsSaveWire {
  return {revision, updatedAt: '2026-09-01T00:00:02Z', fileRevision: revision, settings, restartRequired, problem}
}

async function renderReady(initial = candidate()) {
  const port = new SettingsPortFixture()
  port.gets.push(wire(7, initial))
  const settings = createSettingsModule(port)
  await settings.start()
  const view = render(SettingsPage, {props: {settings}})
  return {port, settings, view}
}

describe('SettingsPage', () => {
  it('routes the real aggregate processing backend field to a focusable summary', async () => {
    const {port, settings} = await renderReady()
    const checking = settings.validate()
    port.validationPending[0]?.resolve(validation(7, candidate(), {
      code: 'validation', field: 'processing', message: '处理配置范围无效。',
    }))
    await checking
    await waitFor(() => expect(screen.getByRole('tab', {name: '处理'})).toHaveAttribute('aria-selected', 'true'))
    await waitFor(() => expect(document.getElementById('processing-summary')).toHaveFocus())
    expect(document.getElementById('processing-summary')).toHaveTextContent('处理配置范围无效。')
  })

  it('disables Save for client errors and standalone validation, then recovers', async () => {
    const {port, settings} = await renderReady()
    const root = screen.getByRole('textbox', {name: 'Avatar OSC 根目录'})
    await fireEvent.input(root, {target: {value: ''}})
    expect(screen.getByRole('button', {name: '保存更改'})).toBeDisabled()
    await fireEvent.blur(root)
    expect(screen.getByRole('alert')).toBeVisible()
    await fireEvent.input(root, {target: {value: 'C:\\corrected'}})
    expect(screen.getByRole('button', {name: '保存更改'})).toBeEnabled()
    const checking = settings.validate()
    await waitFor(() => expect(screen.getByRole('button', {name: '保存更改'})).toBeDisabled())
    port.validationPending[0]?.resolve(validation(7, port.validations[0]!))
    await checking
    await waitFor(() => expect(screen.getByRole('button', {name: '保存更改'})).toBeEnabled())
  })

  it('confirms sticky discard on conflict and keeps the draft when canceled', async () => {
    const {port, settings} = await renderReady()
    settings.updateDraft((draft) => { draft.avatar.oscRoot = 'C:\\mine' })
    port.gets.push(wire(8, candidate({avatar: {oscRoot: 'C:\\theirs', fallbackPath: ''}})))
    await settings.refresh()
    await fireEvent.click(screen.getByRole('button', {name: '放弃更改'}))
    expect(screen.getByRole('dialog', {name: '重新加载设置'})).toBeVisible()
    await fireEvent.click(screen.getByRole('button', {name: '取消'}))
    expect(settings.state.draft?.avatar.oscRoot).toBe('C:\\mine')
    port.gets.push(wire(8, candidate({avatar: {oscRoot: 'C:\\theirs', fallbackPath: ''}})))
    await fireEvent.click(screen.getByRole('button', {name: '放弃更改'}))
    await fireEvent.click(screen.getByRole('button', {name: '确认重新加载'}))
    await waitFor(() => expect(settings.state.draft?.avatar.oscRoot).toBe('C:\\theirs'))
  })

  it('shows retained stale data with its update time after a failed refresh', async () => {
    const {settings} = await renderReady()
    settings.updateDraft((draft) => { draft.avatar.oscRoot = 'C:\\mine' })
    await settings.refresh()
    expect(await screen.findByText(/显示上次可用设置/)).toHaveTextContent('2026-09-01T00:00:00Z')
    expect(screen.getByRole('textbox', {name: 'Avatar OSC 根目录'})).toHaveValue('C:\\mine')
  })

  it.each(['unavailable', 'unsupported_platform'])('shows an owned initial %s Problem', async (code) => {
    const port = new SettingsPortFixture()
    const settings = createSettingsModule(port)
    await settings.start()
    const state = {...settings.state, problem: {code, title: '设置服务状态', detail: `owned ${code}`, tone: 'warning' as const, persistent: true}}
    render(SettingsPage, {props: {settings: {...settings, state}}})
    expect(screen.getByRole('alert')).toHaveTextContent(`owned ${code}`)
  })

  it('reports clean canLeave without changing the draft or making requests', async () => {
    const {view, settings, port} = await renderReady()
    const draft = settings.state.draft
    expect((view.component as unknown as {canLeave(): boolean}).canLeave()).toBe(true)
    expect(settings.state.draft).toBe(draft)
    expect(port.validations).toHaveLength(0)
  })

  it('keeps the draft across tabs, validates on blur, and exposes dirty canLeave state', async () => {
    const {port, settings, view} = await renderReady()

    await fireEvent.input(screen.getByRole('textbox', {name: 'Avatar OSC 根目录'}), {target: {value: 'C:\\New OSC'}})
    await fireEvent.blur(screen.getByRole('textbox', {name: 'Avatar OSC 根目录'}))
    expect(port.validations).toHaveLength(1)
    expect(port.validations[0]?.avatar.oscRoot).toBe('C:\\New OSC')
    port.validationPending[0]?.resolve(validation(7, port.validations[0]!))

    await fireEvent.click(screen.getByRole('tab', {name: '处理'}))
    expect(within(document.getElementById('default-channel')!).getByRole('spinbutton', {name: '中立值'})).toHaveValue(0)
    await fireEvent.click(screen.getByRole('tab', {name: '常规'}))
    expect(screen.getByRole('textbox', {name: 'Avatar OSC 根目录'})).toHaveValue('C:\\New OSC')
    expect(screen.getByRole('region', {name: '未保存的更改'})).toHaveTextContent('有未保存的更改')
    expect((view.component as unknown as {canLeave(): boolean}).canLeave()).toBe(false)
    expect(settings.state.draft?.avatar.oscRoot).toBe('C:\\New OSC')
  })

  it('edits the active-channel stale timeout through the processing draft and validates its owning field', async () => {
    const {port, settings} = await renderReady()
    const validate = vi.spyOn(settings, 'validate')

    await fireEvent.click(screen.getByRole('tab', {name: '处理'}))
    const staleTimeout = screen.getByRole('spinbutton', {name: '活跃通道过期时长（毫秒）'})
    await fireEvent.input(staleTimeout, {target: {value: '2500'}})
    await fireEvent.blur(staleTimeout)

    expect(settings.state.draft?.processing.activeStaleAfterMs).toBe(2500)
    expect(validate).toHaveBeenCalledWith('processing.activeStaleAfterMs')
    expect(port.validations.at(-1)?.processing.activeStaleAfterMs).toBe(2500)
  })

  it('renders, routes, and focuses an active stale timeout field Problem independently of default channel settings', async () => {
    const {port, settings} = await renderReady()
    const checking = settings.validate('processing.activeStaleAfterMs')
    port.validationPending[0]?.resolve(validation(7, candidate(), {
      code: 'validation', message: '活跃通道过期时长无效。', field: 'processing.activeStaleAfterMs',
    }))
    await checking

    await waitFor(() => expect(screen.getByRole('tab', {name: '处理'})).toHaveAttribute('aria-selected', 'true'))
    await waitFor(() => expect(screen.getByRole('spinbutton', {name: '活跃通道过期时长（毫秒）'})).toHaveFocus())
    expect(screen.getByText('活跃通道过期时长无效。')).toBeVisible()
    expect(document.getElementById('default-channel')).not.toHaveFocus()
  })

  it('validates repeated editor fields when focus leaves their owning group', async () => {
    const {port} = await renderReady()

    await fireEvent.focusOut(screen.getByRole('textbox', {name: '开发目录 0'}))

    expect(port.validations).toHaveLength(1)
    expect(port.validations[0]?.plugins.devRoots).toEqual(['C:\\plugins'])
  })

  it('disables the irrelevant OSC mode controls with explanations while preserving their draft values', async () => {
    const {settings} = await renderReady(candidate({
      osc: {targetMode: 'manual', preferredService: 'VRChat-Client', manualHost: '127.0.0.1', manualPort: 9001},
    }))

    await fireEvent.click(screen.getByRole('tab', {name: 'OSC'}))
    expect(screen.getByRole('button', {name: '目标模式'})).toHaveAttribute('id', 'osc-target-mode')
    expect(screen.getByRole('textbox', {name: '首选发现服务'})).toBeDisabled()
    expect(screen.getByText('仅自动模式可编辑首选发现服务。')).toBeVisible()
    expect(screen.getByRole('textbox', {name: '手动主机'})).toBeEnabled()
    expect(screen.getByRole('spinbutton', {name: '手动端口'})).toHaveValue(9001)

    await fireEvent.pointerDown(screen.getByRole('button', {name: '目标模式'}), {button: 0, ctrlKey: false})
    const automatic = screen.getByRole('option', {name: '自动'})
    await fireEvent.pointerDown(automatic, {button: 0, ctrlKey: false})
    await fireEvent.pointerUp(automatic, {button: 0, ctrlKey: false})

    expect(screen.getByRole('textbox', {name: '首选发现服务'})).toBeEnabled()
    expect(screen.getByRole('textbox', {name: '手动主机'})).toHaveAccessibleDescription('仅手动模式可编辑主机和端口。')
    expect(screen.getByRole('spinbutton', {name: '手动端口'})).toHaveAccessibleDescription('仅手动模式可编辑主机和端口。')
    expect(settings.state.draft?.osc.manualPort).toBe(9001)
  })

  it('validates enabled preferred service on blur and routes its backend Problem to the mapped target', async () => {
    const {port, settings} = await renderReady()
    const validate = vi.spyOn(settings, 'validate')

    await fireEvent.click(screen.getByRole('tab', {name: 'OSC'}))
    const preferredService = screen.getByRole('textbox', {name: '首选发现服务'})
    await fireEvent.blur(preferredService)
    expect(validate).toHaveBeenCalledWith('osc.preferredService')
    port.validationPending[0]?.resolve(validation(7, candidate(), {
      code: 'validation', message: '首选服务不可用。', field: 'osc.preferredService',
    }))

    await waitFor(() => expect(document.getElementById('osc-preferred-service')).toHaveFocus())
    expect(screen.getByText('首选服务不可用。')).toBeVisible()
    expect(preferredService).toBeEnabled()
  })

  it('routes a manual-mode preferred-service client error from another tab to a focusable mapped target', async () => {
    const {port, settings} = await renderReady(candidate({
      osc: {targetMode: 'manual', preferredService: 'VRChat-Client', manualHost: '127.0.0.1', manualPort: 9001},
    }))

    const checking = settings.validate('osc.preferredService')
    expect(port.validationPending).toHaveLength(0)
    expect(await checking).toBe(false)

    await waitFor(() => expect(screen.getByRole('tab', {name: 'OSC'})).toHaveAttribute('aria-selected', 'true'))
    const target = document.getElementById('osc-preferred-service')
    const preferredService = screen.getByRole('textbox', {name: '首选发现服务'})
    await waitFor(() => expect(target).toHaveFocus())
    expect(target).toHaveAttribute('tabindex', '-1')
    expect(preferredService).toHaveAttribute('id', 'osc-preferred-service-input')
    expect(preferredService).toBeDisabled()
    expect(preferredService).toHaveValue('VRChat-Client')
    expect(screen.getByText('手动模式不能设置首选发现服务。')).toBeVisible()
    expect(settings.state.draft?.osc.preferredService).toBe('VRChat-Client')
  })

  it('routes a backend field Problem to its owning tab and exact focus target without dropping the draft', async () => {
    const {port, settings} = await renderReady(candidate({
      osc: {targetMode: 'manual', preferredService: '', manualHost: '127.0.0.1', manualPort: 9001},
    }))
    settings.updateDraft((draft) => { draft.avatar.fallbackPath = 'C:\\mine.json' })
    const checking = settings.validate('osc.manualPort')
    port.validationPending[0]?.resolve(validation(7, candidate(), {
      code: 'validation', message: '端口不可用。', field: 'osc.manualPort',
    }))
    await checking

    await waitFor(() => expect(screen.getByRole('tab', {name: 'OSC'})).toHaveAttribute('aria-selected', 'true'))
    await waitFor(() => expect(screen.getByRole('spinbutton', {name: '手动端口'})).toHaveFocus())
    expect(screen.getByText('端口不可用。')).toBeVisible()
    expect(settings.state.draft?.avatar.fallbackPath).toBe('C:\\mine.json')
  })

  it('shows a pending save once, then keeps the restart-required confirmation visible after success', async () => {
    const {port} = await renderReady()
    await fireEvent.input(screen.getByRole('textbox', {name: 'Avatar OSC 根目录'}), {target: {value: 'C:\\Saved OSC'}})
    const saveButton = screen.getByRole('button', {name: '保存更改'})
    await fireEvent.click(saveButton)
    await fireEvent.click(saveButton)

    expect(saveButton).toBeDisabled()
    expect(port.validations).toHaveLength(1)
    port.validationPending[0]?.resolve(validation(7, port.validations[0]!))
    await waitFor(() => expect(port.saves).toHaveLength(1))
    port.savePending[0]?.resolve(save(8, port.saves[0]!.candidate, true))

    expect(await screen.findByText('已保存，将在重启后生效')).toBeVisible()
  })

  it('preserves a conflicted draft until the user confirms reload, while cancel leaves it intact', async () => {
    const {port, settings} = await renderReady()
    await fireEvent.input(screen.getByRole('textbox', {name: 'Avatar OSC 根目录'}), {target: {value: 'C:\\Mine'}})
    await fireEvent.click(screen.getByRole('button', {name: '保存更改'}))
    port.validationPending[0]?.resolve(validation(7, port.validations[0]!))
    await waitFor(() => expect(port.saves).toHaveLength(1))
    port.savePending[0]?.resolve(save(8, candidate(), false, {code: 'conflict', message: '设置已被更新。', currentRevision: 8}))

    const reload = await screen.findByRole('button', {name: '重新加载设置'})
    await fireEvent.click(reload)
    await fireEvent.click(screen.getByRole('button', {name: '取消'}))
    expect(settings.state.draft?.avatar.oscRoot).toBe('C:\\Mine')

    port.gets.push(wire(8, candidate({avatar: {oscRoot: 'C:\\Reloaded', fallbackPath: 'C:\\VRChat\\fallback.json'}})))
    await fireEvent.click(reload)
    await fireEvent.click(screen.getByRole('button', {name: '确认重新加载'}))
    await waitFor(() => expect(settings.state.draft?.avatar.oscRoot).toBe('C:\\Reloaded'))
  })
})
