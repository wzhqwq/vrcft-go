import {copy as zhCN} from './zh-CN.js'
import {copy as enUS} from './en-US.js'

export type LanguagePreference = 'system' | 'en-US' | 'zh-CN'
export type Locale = 'en-US' | 'zh-CN'

const storageKey = 'vrcft-go.language'

function validPreference(value: unknown): value is LanguagePreference {
  return value === 'system' || value === 'en-US' || value === 'zh-CN'
}

function storedPreference(): LanguagePreference {
  try {
    const stored = localStorage.getItem(storageKey)
    return validPreference(stored) ? stored : 'system'
  } catch {
    return 'system'
  }
}

let preference = $state<LanguagePreference>(storedPreference())
let systemLanguage = $state(typeof navigator === 'undefined' ? '' : navigator.language)

export function resolveLanguage(selected: LanguagePreference, system: string): Locale {
  if (selected !== 'system') return selected
  return /^zh(?:$|-(?:Hans|CN|SG)(?:-|$))/i.test(system) ? 'zh-CN' : 'en-US'
}

export function getLanguagePreference(): LanguagePreference {
  return preference
}

export function getLocale(): Locale {
  return resolveLanguage(preference, systemLanguage)
}

export function setLanguagePreference(value: LanguagePreference): void {
  if (!validPreference(value)) return
  try {
    localStorage.setItem(storageKey, value)
  } catch {
    // The current session can still switch language when storage is unavailable.
  }
  preference = value
  if (typeof document !== 'undefined') document.documentElement.lang = getLocale()
}

export function refreshSystemLanguage(): void {
  systemLanguage = typeof navigator === 'undefined' ? '' : navigator.language
  if (typeof document !== 'undefined') document.documentElement.lang = getLocale()
}

if (typeof document !== 'undefined') document.documentElement.lang = getLocale()
if (typeof window !== 'undefined') window.addEventListener('languagechange', refreshSystemLanguage)

// Reading getLocale() inside each nested property access makes Svelte track language
// changes even in components that were mounted before the preference changed.
function localized<T extends object>(path: PropertyKey[] = []): T {
  const current = () => {
    const source = getLocale() === 'zh-CN' ? zhCN : enUS
    return path.reduce<unknown>((node, part) => (node as Record<PropertyKey, unknown>)[part], source) as Record<PropertyKey, unknown>
  }
  return new Proxy({} as T, {
    get(_target, key) {
      const value = current()[key]
      return value !== null && typeof value === 'object' ? localized([...path, key]) : value
    },
    has(_target, key) { return key in current() },
    ownKeys() { return Reflect.ownKeys(current()) },
    getOwnPropertyDescriptor(_target, key) {
      return key in current() ? {configurable: true, enumerable: true} : undefined
    },
  })
}

export const copy = localized<typeof zhCN>()
