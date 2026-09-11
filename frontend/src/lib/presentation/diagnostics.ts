/** Retain actionable error text, but never credentials or local private paths. */
export function diagnosticLogPath(value: string): string {
  if (/[\\/]vrcft-go[\\/]logs[\\/]application\.jsonl$/i.test(value)) return String.raw`%APPDATA%\vrcft-go\logs\application.jsonl`
  return diagnosticText(value)
}

export function diagnosticText(value: unknown, limit = 1024): string {
  const text = value instanceof Error ? value.message : typeof value === 'string' ? value : '未知错误'
  return text
    .replace(/\b((?:proxy-)?authorization|(?:set-)?cookie)\s*:\s*[^\r\n]*/gi, '$1: [REDACTED]')
    .replace(/(https?:\/\/)[^\s/@]+@/gi, '$1[REDACTED]@')
    .replace(/\b(Bearer)\s+[^\s,;]+/gi, '$1 [REDACTED]')
    .replace(/(["']?(?:password|passwd|token|secret|api[_-]?key|authorization|cookie|sessionId)["']?\s*[:=]\s*)(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|[^\s,;}]+)/gi, '$1[REDACTED]')
    .replace(/\b[A-Za-z]:[\\/]Users[\\/][^:\r\n"'<>|]+/gi, '[PATH]')
    .replace(/\b[A-Za-z]:[\\/][^\s"'<>|]+/g, '[PATH]')
    .replace(/\/(?:home|Users)\/[^:\r\n"'<>|]+/g, '[PATH]')
    .replace(/[\u0000-\u001f\u007f]/g, ' ')
    .trim().slice(0, limit)
}
