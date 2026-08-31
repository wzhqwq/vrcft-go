import type {
  PluginListWire,
  PluginMutationWire,
  RuntimeWire,
  SettingsCandidate,
  SettingsSaveWire,
  SettingsValidationWire,
  SettingsWire,
} from './types'

export type Stop = () => void

export interface RuntimePort {
  getStatus(): Promise<RuntimeWire>
  onChanged(listener: (value: unknown) => void): Stop
}

export interface PluginsPort {
  list(): Promise<PluginListWire>
  setEnabled(pluginId: string, enabled: boolean): Promise<PluginMutationWire>
  onChanged(listener: (value: unknown) => void): Stop
}

export interface SettingsPort {
  get(): Promise<SettingsWire>
  validate(candidate: SettingsCandidate): Promise<SettingsValidationWire>
  save(expectedRevision: number, candidate: SettingsCandidate): Promise<SettingsSaveWire>
  onChanged(listener: (value: unknown) => void): Stop
}

export interface WailsPorts {
  runtime: RuntimePort
  plugins: PluginsPort
  settings: SettingsPort
}

export interface MockPortEvents {
  runtime(value: unknown): void
  plugins(value: unknown): void
  settings(value: unknown): void
}

export interface ListenerPort<T> {
  emit(value: T): void
  onChanged(listener: (value: T) => void): Stop
}

export function createListenerPort<T>(connect?: (emit: (value: T) => void) => Stop): ListenerPort<T> {
  const listeners = new Map<number, (value: T) => void>()
  let nextID = 0
  let disconnect: Stop | null = null

  const emit = (value: T) => {
    for (const listener of [...listeners.values()]) {
      listener(value)
    }
  }

  return {
    emit,
    onChanged(listener) {
      if (listeners.size === 0 && connect !== undefined) {
        disconnect = connect(emit)
      }

      const id = nextID
      nextID += 1
      listeners.set(id, listener)

      let stopped = false

      return () => {
        if (stopped) {
          return
        }

        stopped = true
        listeners.delete(id)

        if (listeners.size === 0 && disconnect !== null) {
          const stop = disconnect
          disconnect = null
          stop()
        }
      }
    },
  }
}
