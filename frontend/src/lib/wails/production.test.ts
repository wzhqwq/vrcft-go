import {beforeEach, describe, expect, it, vi} from 'vitest'

const runtimeMock = vi.hoisted(() => {
  const listeners = new Map<string, Set<(...values: unknown[]) => void>>()

  const on = vi.fn((eventName: string, callback: (...values: unknown[]) => void) => {
    let registered = listeners.get(eventName)
    if (registered === undefined) {
      registered = new Set()
      listeners.set(eventName, registered)
    }
    registered.add(callback)

    return () => {
      const current = listeners.get(eventName)
      current?.delete(callback)
      if (current?.size === 0) {
        listeners.delete(eventName)
      }
    }
  })

  const off = vi.fn((eventName: string) => {
    listeners.delete(eventName)
  })

  const emit = (eventName: string, value: unknown) => {
    for (const listener of listeners.get(eventName) ?? []) {
      listener(value)
    }
  }

  const reset = () => {
    listeners.clear()
    on.mockClear()
    off.mockClear()
  }

  return {emit, off, on, reset}
})

vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOff: runtimeMock.off,
  EventsOn: runtimeMock.on,
}))

vi.mock('../../../wailsjs/go/main/RuntimeAPI', () => ({
  GetStatus: vi.fn(),
}))

vi.mock('../../../wailsjs/go/main/PluginsAPI', () => ({
  List: vi.fn(),
  SetEnabled: vi.fn(),
}))

vi.mock('../../../wailsjs/go/main/SettingsAPI', () => ({
  Get: vi.fn(),
  Save: vi.fn(),
  Validate: vi.fn(),
}))

describe('productionPorts', () => {
  beforeEach(() => {
    runtimeMock.reset()
    vi.resetModules()
  })

  it('stops only the adapter runtime listener and preserves other event consumers', async () => {
    const externalListener = vi.fn()
    runtimeMock.on('vrcft:v1:runtime-status', externalListener)

    const {productionPorts} = await import('./production')
    const ports = productionPorts()
    const adapterListener = vi.fn()

    const stop = ports.runtime.onChanged(adapterListener)
    stop()
    runtimeMock.emit('vrcft:v1:runtime-status', {revision: 2})

    expect(adapterListener).not.toHaveBeenCalled()
    expect(externalListener).toHaveBeenCalledOnce()
    expect(runtimeMock.off).not.toHaveBeenCalled()
  })
})
