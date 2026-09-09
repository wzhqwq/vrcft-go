/** Commands receive an already allowlisted display/copy value, never a backend object. */
export function copyText(text: string): void {
  void navigator.clipboard?.writeText(text).catch(() => undefined)
}
