import {describe, expect, it} from 'vitest'
import {diagnosticText, diagnosticLogPath} from './diagnostics.js'
import {localTime} from './time.js'

describe('diagnostic presentation', () => {
  it('retains the known log destination without disclosing a profile path', () => {
    expect(diagnosticLogPath(String.raw`C:\Users\Jane Doe\AppData\Roaming\vrcft-go\logs\application.jsonl`)).toBe(String.raw`%APPDATA%\vrcft-go\logs\application.jsonl`)
  })
  it('formats valid timestamps in the operating system timezone and handles invalid dates', () => {
    const time = '2026-09-10T04:30:00Z'
    expect(localTime(time)).toBe(new Intl.DateTimeFormat('zh-CN', {dateStyle: 'short', timeStyle: 'medium'}).format(new Date(time)))
    expect(localTime(null)).toBe('尚未更新')
    expect(localTime('bad date')).toBe('时间无效')
  })
  it('preserves useful errors while removing paths, credentials, control characters and unbounded text', () => {
    const value = diagnosticText('listen udp :9001: address already in use\nC:/Users/alice/private.json token=secret password="two words" Bearer credential')
    expect(value).toContain('listen udp :9001: address already in use')
    expect(value).not.toMatch(/alice|private.json|secret|two words|credential|\n/)
    expect(diagnosticText('x'.repeat(2000))).toHaveLength(1024)
  })
  it.each([
    [String.raw`open C:\Users\Jane Doe\AppData\config.json: Access is denied.`, 'Access is denied.'],
    ['open /home/Jane Doe/config.json: permission denied', 'permission denied'],
  ])('retains actionable reasons after redacted private paths in %s', (input, reason) => {
    const result = diagnosticText(input)
    expect(result).toContain(reason)
    expect(result).not.toMatch(/Jane|Doe|config\.json/)
  })
  it.each([
    ['Authorization: Basic dXNlcjpwYXNz', 'dXNlcjpwYXNz'],
    ['Cookie: session=private; csrf=private2', 'private'],
    [String.raw`{"password":"first\"secret_tail"}`, 'secret_tail'],
    [String.raw`open C:\Users\Jane Doe\private.json failed`, 'Doe'],
    ['connect https://user:password@host failed', 'user:password'],
  ])('redacts compound credentials and profile paths from %s', (input, secret) => {
    expect(diagnosticText(input)).not.toContain(secret)
  })
  it('preserves the request failure surrounding a cookie assignment', () => {
    expect(diagnosticText('request failed cookie=private_session')).toBe('request failed cookie=[REDACTED]')
  })
})
