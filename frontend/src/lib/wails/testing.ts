import {createListenerPort, type MockPortEvents, type WailsPorts} from './ports'
import type {
  PluginListWire,
  PluginMutationWire,
  RuntimeWire,
  SettingsCandidate,
  SettingsSaveWire,
  SettingsValidationWire,
  SettingsWire,
} from './types'

type Queue<T> = T[]

interface MockPortCalls {
  runtime: {
    getStatus: number
  }
  plugins: {
    list: number
    setEnabled: Array<{pluginId: string; enabled: boolean}>
  }
  settings: {
    get: number
    validate: SettingsCandidate[]
    save: Array<{expectedRevision: number; candidate: SettingsCandidate}>
  }
}

interface MockPortQueue {
  runtimeStatus(...values: RuntimeWire[]): void
  pluginLists(...values: PluginListWire[]): void
  pluginMutations(...values: PluginMutationWire[]): void
  settings(...values: SettingsWire[]): void
  settingsValidations(...values: SettingsValidationWire[]): void
  settingsSaves(...values: SettingsSaveWire[]): void
}

export interface MockPorts extends WailsPorts {
  calls: MockPortCalls
  events: MockPortEvents
  queue: MockPortQueue
}

export function createMockPorts(): MockPorts {
  const runtimeChanges = createListenerPort<unknown>()
  const pluginChanges = createListenerPort<unknown>()
  const settingsChanges = createListenerPort<unknown>()

  const runtimeQueue: Queue<RuntimeWire> = []
  const pluginListQueue: Queue<PluginListWire> = []
  const pluginMutationQueue: Queue<PluginMutationWire> = []
  const settingsQueue: Queue<SettingsWire> = []
  const settingsValidationQueue: Queue<SettingsValidationWire> = []
  const settingsSaveQueue: Queue<SettingsSaveWire> = []

  const calls: MockPortCalls = {
    runtime: {
      getStatus: 0,
    },
    plugins: {
      list: 0,
      setEnabled: [],
    },
    settings: {
      get: 0,
      validate: [],
      save: [],
    },
  }

  return {
    runtime: {
      async getStatus() {
        calls.runtime.getStatus += 1
        return shiftQueued('runtime.getStatus', runtimeQueue)
      },
      onChanged(listener) {
        return runtimeChanges.onChanged(listener)
      },
    },
    plugins: {
      async list() {
        calls.plugins.list += 1
        return shiftQueued('plugins.list', pluginListQueue)
      },
      async setEnabled(pluginId, enabled) {
        calls.plugins.setEnabled.push({pluginId, enabled})
        return shiftQueued('plugins.setEnabled', pluginMutationQueue)
      },
      onChanged(listener) {
        return pluginChanges.onChanged(listener)
      },
    },
    settings: {
      async get() {
        calls.settings.get += 1
        return shiftQueued('settings.get', settingsQueue)
      },
      async validate(candidate) {
        calls.settings.validate.push(candidate)
        return shiftQueued('settings.validate', settingsValidationQueue)
      },
      async save(expectedRevision, candidate) {
        calls.settings.save.push({expectedRevision, candidate})
        return shiftQueued('settings.save', settingsSaveQueue)
      },
      onChanged(listener) {
        return settingsChanges.onChanged(listener)
      },
    },
    calls,
    events: {
      runtime(value) {
        runtimeChanges.emit(value)
      },
      plugins(value) {
        pluginChanges.emit(value)
      },
      settings(value) {
        settingsChanges.emit(value)
      },
    },
    queue: {
      runtimeStatus(...values) {
        runtimeQueue.push(...values)
      },
      pluginLists(...values) {
        pluginListQueue.push(...values)
      },
      pluginMutations(...values) {
        pluginMutationQueue.push(...values)
      },
      settings(...values) {
        settingsQueue.push(...values)
      },
      settingsValidations(...values) {
        settingsValidationQueue.push(...values)
      },
      settingsSaves(...values) {
        settingsSaveQueue.push(...values)
      },
    },
  }
}

function shiftQueued<T>(label: string, queue: Queue<T>): T {
  const next = queue.shift()

  if (next === undefined) {
    throw new Error(`No queued response for ${label}`)
  }

  return next
}
