export function localTime(value: string | null | undefined): string {
  if (!value) return '尚未更新'
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return '时间无效'
  return new Intl.DateTimeFormat('zh-CN', {dateStyle: 'short', timeStyle: 'medium'}).format(date)
}
