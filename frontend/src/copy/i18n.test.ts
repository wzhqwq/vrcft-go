import {beforeEach, describe, expect, it, vi} from 'vitest'
import {copy as chineseCopy} from './zh-CN.js'
import {copy as englishCopy} from './en-US.js'

function shape(value: unknown): unknown {
  if (value !== null && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value).map(([key, child]) => [key, shape(child)]))
  }
  return typeof value
}

describe('frontend language', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.resetModules()
  })

  it('has matching Chinese and English message keys and value types', () => {
    expect(shape(englishCopy)).toEqual(shape(chineseCopy))
  })

  it('follows Chinese system language and falls back to English for unsupported languages', async () => {
    Object.defineProperty(navigator, 'language', {configurable: true, value: 'zh-CN'})
    const chinese = await import('./index.svelte.js')
    expect(chinese.resolveLanguage('system', 'zh-CN')).toBe('zh-CN')
    expect(chinese.resolveLanguage('system', 'ja-JP')).toBe('en-US')
    expect(chinese.resolveLanguage('system', 'zh-TW')).toBe('en-US')
    expect(chinese.resolveLanguage('system', 'en-GB')).toBe('en-US')
    Object.defineProperty(navigator, 'language', {configurable: true, value: 'ja-JP'})
    chinese.refreshSystemLanguage()
    expect(chinese.getLocale()).toBe('en-US')
    expect(chinese.copy.navigation.settings).toBe('Settings')
    expect(document.documentElement.lang).toBe('en-US')
  })

  it('persists manual selection and ignores invalid stored preferences', async () => {
    const i18n = await import('./index.svelte.js')
    i18n.setLanguagePreference('en-US')
    expect(localStorage.getItem('vrcft-go.language')).toBe('en-US')
    expect(i18n.copy.navigation.settings).toBe('Settings')
    i18n.setLanguagePreference('zh-CN')
    expect(i18n.copy.navigation.settings).toBe('设置')
    expect(document.documentElement.lang).toBe('zh-CN')
    localStorage.setItem('vrcft-go.language', 'invalid')
    vi.resetModules()
    const reloaded = await import('./index.svelte.js')
    expect(reloaded.getLanguagePreference()).toBe('system')
  })
})
