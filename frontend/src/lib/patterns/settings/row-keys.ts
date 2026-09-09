/** Mounted editor row identity is independent of editable text and array position. */
export function createRowKeys() {
  const keys: number[] = []
  let next = 0
  return {
    at(index: number): number {
      while (keys.length <= index) keys.push(next++)
      return keys[index]!
    },
    remove(index: number) { keys.splice(index, 1) },
  }
}
