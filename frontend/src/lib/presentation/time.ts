import {getLocale} from '../../copy/index.svelte.js'

export function localTime(value: string | null | undefined): string {
  if (!value) return getLocale() === 'zh-CN' ? '尚未更新' : 'Not updated'
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return getLocale() === 'zh-CN' ? '时间无效' : 'Invalid time'
  return new Intl.DateTimeFormat(getLocale(), {dateStyle: 'short', timeStyle: 'medium'}).format(date)
}
