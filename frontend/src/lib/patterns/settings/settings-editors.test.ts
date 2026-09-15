import {fireEvent, render, screen} from '@testing-library/svelte'
import {describe, expect, it} from 'vitest'

import ChannelOverridesEditor from './ChannelOverridesEditor.svelte'
import MutualExclusionEditor from './MutualExclusionEditor.svelte'
import PathListField from './PathListField.svelte'
import ProcessingChannelFields from './ProcessingChannelFields.svelte'
import type {ProcessingChannel} from '../../modules/settings/form.js'

const channel: ProcessingChannel = {
  calibration: {enabled: true, neutral: 0, min: -1, max: 1, gain: 1, invert: false},
  tuning: {deadzone: 0, gain: 1, exponent: 1, clampEnabled: false, clampMin: -1, clampMax: 1},
  filter: {mode: 'ema', emaAlpha: 0.5, minCutoff: 1, beta: 0, derivativeCutoff: 1},
  dropout: {holdDurationMs: 10, decayDurationMs: 20, staleAfterMs: 30},
}

describe('settings repeated editors', () => {
  it('emits cloned development roots for add, edit, and remove', async () => {
    const roots = Object.freeze(['C:\\plugins'])
    const added: string[][] = []
    const addedView = render(PathListField, {props: {values: roots, onChange: (value) => added.push(value)}})
    await fireEvent.click(screen.getByRole('button', {name: '添加开发目录'}))
    expect(added).toEqual([['C:\\plugins', '']])
    addedView.unmount()

    const edited: string[][] = []
    const editedView = render(PathListField, {props: {values: roots, onChange: (value) => edited.push(value)}})
    await fireEvent.input(screen.getByRole('textbox', {name: '开发目录 0'}), {target: {value: 'D:\\plugins'}})
    expect(edited).toEqual([['D:\\plugins']])
    editedView.unmount()

    const removed: string[][] = []
    render(PathListField, {props: {values: roots, onChange: (value) => removed.push(value)}})
    await fireEvent.click(screen.getByRole('button', {name: '删除开发目录 0'}))
    expect(removed).toEqual([[]])
    expect(roots).toEqual(['C:\\plugins'])
  })

  it('emits cloned nested calibration, tuning, filter, and dropout channel values', async () => {
    const frozen = Object.freeze(structuredClone(channel))
    const changes: ProcessingChannel[] = []
    render(ProcessingChannelFields, {props: {value: frozen, onChange: (value) => changes.push(value)}})

    await fireEvent.click(screen.getByRole('switch', {name: '启用输入校准'}))
    expect(changes.at(-1)?.calibration.enabled).toBe(false)
    await fireEvent.input(screen.getByRole('spinbutton', {name: '静止基准值'}), {target: {value: '0.25'}})
    expect(changes.at(-1)?.calibration.neutral).toBe(0.25)
    await fireEvent.click(screen.getByRole('tab', {name: '响应调节'}))
    await fireEvent.input(screen.getByRole('spinbutton', {name: '忽略微小输入'}), {target: {value: '0.1'}})
    expect(changes.at(-1)?.tuning.deadzone).toBe(0.1)
    await fireEvent.click(screen.getByRole('tab', {name: '平滑处理'}))
    await fireEvent.input(screen.getByRole('spinbutton', {name: 'EMA 响应速度'}), {target: {value: '0.2'}})
    expect(changes.at(-1)?.filter.emaAlpha).toBe(0.2)
    await fireEvent.click(screen.getByRole('tab', {name: '信号丢失处理'}))
    await fireEvent.input(screen.getByRole('spinbutton', {name: '保持最后值时长（毫秒）'}), {target: {value: '11'}})
    expect(changes.at(-1)?.dropout.holdDurationMs).toBe(11)
    expect(frozen).toEqual(channel)
  })

  it('uses vertical tabs to show one processing stage at a time', async () => {
    render(ProcessingChannelFields, {props: {value: channel, onChange: () => {}}})

    expect(screen.getByRole('tablist')).toHaveAttribute('aria-orientation', 'vertical')
    expect(screen.getByRole('tab', {name: '输入校准'})).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('spinbutton', {name: '静止基准值'})).toBeVisible()
    expect(screen.queryByRole('spinbutton', {name: '忽略微小输入'})).not.toBeInTheDocument()

    await fireEvent.click(screen.getByRole('tab', {name: '响应调节'}))
    expect(screen.getByRole('spinbutton', {name: '忽略微小输入'})).toBeVisible()
    expect(screen.queryByRole('spinbutton', {name: '静止基准值'})).not.toBeInTheDocument()
  })

  it('emits cloned channel overrides for add, name edit, nested edit, and removal', async () => {
    const values = Object.freeze([{name: 'Eye', channel: structuredClone(channel)}])
    const added: Array<{name: string; channel: ProcessingChannel}[]> = []
    const addedView = render(ChannelOverridesEditor, {props: {defaultChannel: channel, values, onDefaultChange: () => {}, onChange: (value) => added.push(value)}})
    await fireEvent.click(screen.getByRole('button', {name: '添加自定义处理'}))
    expect(added).toHaveLength(1)
    expect(added[0]).toHaveLength(2)
    addedView.unmount()

    const changed: Array<{name: string; channel: ProcessingChannel}[]> = []
    const changedView = render(ChannelOverridesEditor, {props: {defaultChannel: channel, values, onDefaultChange: () => {}, onChange: (value) => changed.push(value)}})
    const changedSelector = screen.getByRole('button', {name: '处理配置'})
    await fireEvent.pointerDown(changedSelector, {button: 0, ctrlKey: false})
    const changedOption = screen.getByRole('option', {name: '自定义：Eye'})
    await fireEvent.pointerDown(changedOption, {button: 0, ctrlKey: false})
    await fireEvent.pointerUp(changedOption, {button: 0, ctrlKey: false})
    await fireEvent.input(screen.getByRole('textbox', {name: '覆盖通道名称'}), {target: {value: 'Mouth'}})
    expect(changed.at(-1)?.[0].name).toBe('Mouth')
    await fireEvent.input(screen.getByRole('spinbutton', {name: '静止基准值'}), {target: {value: '0.4'}})
    expect(changed.at(-1)?.[0].channel.calibration.neutral).toBe(0.4)
    changedView.unmount()

    const removed: Array<{name: string; channel: ProcessingChannel}[]> = []
    render(ChannelOverridesEditor, {props: {defaultChannel: channel, values, onDefaultChange: () => {}, onChange: (value) => removed.push(value)}})
    const removedSelector = screen.getByRole('button', {name: '处理配置'})
    await fireEvent.pointerDown(removedSelector, {button: 0, ctrlKey: false})
    const removedOption = screen.getByRole('option', {name: '自定义：Eye'})
    await fireEvent.pointerDown(removedOption, {button: 0, ctrlKey: false})
    await fireEvent.pointerUp(removedOption, {button: 0, ctrlKey: false})
    await fireEvent.click(screen.getByRole('button', {name: '删除自定义处理'}))
    expect(removed).toEqual([[]])
    expect(values[0].channel.calibration.neutral).toBe(0)
  })

  it('emits cloned mutual-exclusion groups for add, edit, and remove', async () => {
    const values = Object.freeze([Object.freeze(['Eye', 'Mouth'])])
    const added: string[][][] = []
    const addedView = render(MutualExclusionEditor, {props: {values, onChange: (value) => added.push(value)}})
    await fireEvent.click(screen.getByRole('button', {name: '添加互斥组'}))
    expect(added).toEqual([[['Eye', 'Mouth'], []]])
    addedView.unmount()

    const edited: string[][][] = []
    const editedView = render(MutualExclusionEditor, {props: {values, onChange: (value) => edited.push(value)}})
    await fireEvent.input(screen.getByRole('textbox', {name: '互斥组 0 成员'}), {target: {value: 'Brow, Cheek'}})
    expect(edited).toEqual([[['Brow', 'Cheek']]])
    editedView.unmount()

    const removed: string[][][] = []
    render(MutualExclusionEditor, {props: {values, onChange: (value) => removed.push(value)}})
    await fireEvent.click(screen.getByRole('button', {name: '删除互斥组 0'}))
    expect(removed).toEqual([[]])
    expect(values).toEqual([['Eye', 'Mouth']])
  })

  it('makes each supplied field target focusable for backend problem routing', () => {
    const pathList = render(PathListField, {props: {id: 'plugin-dev-roots', values: [], onChange: () => {}}})
    const pathTarget = pathList.container.querySelector('#plugin-dev-roots') as HTMLElement
    pathTarget.focus()
    expect(pathTarget).toHaveFocus()
    pathList.unmount()

    const channelFields = render(ProcessingChannelFields, {props: {id: 'default-channel', value: channel, onChange: () => {}}})
    const channelTarget = channelFields.container.querySelector('#default-channel') as HTMLElement
    channelTarget.focus()
    expect(channelTarget).toHaveFocus()
    channelFields.unmount()

    const overrides = render(ChannelOverridesEditor, {props: {id: 'channel-overrides', defaultChannel: channel, values: [], onDefaultChange: () => {}, onChange: () => {}}})
    const overrideTarget = overrides.container.querySelector('#channel-overrides') as HTMLElement
    overrideTarget.focus()
    expect(overrideTarget).toHaveFocus()
    overrides.unmount()

    const exclusions = render(MutualExclusionEditor, {props: {id: 'mutual-exclusion', values: [], onChange: () => {}}})
    const exclusionTarget = exclusions.container.querySelector('#mutual-exclusion') as HTMLElement
    exclusionTarget.focus()
    expect(exclusionTarget).toHaveFocus()
  })
})
