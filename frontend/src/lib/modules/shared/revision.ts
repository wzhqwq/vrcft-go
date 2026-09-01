export function acceptRevision(current: number, candidate: number): boolean {
  return Number.isSafeInteger(candidate) && candidate >= current
}
