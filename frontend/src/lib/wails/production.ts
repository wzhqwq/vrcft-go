import {EventsOff, EventsOn} from '../../../wailsjs/runtime/runtime'
import {List, SetEnabled} from '../../../wailsjs/go/main/PluginsAPI'
import {GetStatus} from '../../../wailsjs/go/main/RuntimeAPI'
import {Get, Save, Validate} from '../../../wailsjs/go/main/SettingsAPI'

import {createListenerPort, type WailsPorts} from './ports'
import type {PluginListWire, PluginMutationWire, RuntimeWire, SettingsCandidate, SettingsSaveWire, SettingsValidationWire, SettingsWire} from './types'

const runtimeChanged = createListenerPort<unknown>((emit) => subscribe('vrcft:v1:runtime-status', emit))
const pluginsChanged = createListenerPort<unknown>((emit) => subscribe('vrcft:v1:plugins-changed', emit))
const settingsChanged = createListenerPort<unknown>((emit) => subscribe('vrcft:v1:settings-changed', emit))

export function productionPorts(): WailsPorts {
  return {
    runtime: {
      getStatus() {
        return GetStatus() as Promise<RuntimeWire>
      },
      onChanged(listener) {
        return runtimeChanged.onChanged(listener)
      },
    },
    plugins: {
      list() {
        return List() as Promise<PluginListWire>
      },
      setEnabled(pluginId, enabled) {
        return SetEnabled(pluginId, enabled) as Promise<PluginMutationWire>
      },
      onChanged(listener) {
        return pluginsChanged.onChanged(listener)
      },
    },
    settings: {
      get() {
        return Get() as Promise<SettingsWire>
      },
      validate(candidate: SettingsCandidate) {
        return Validate(candidate as Parameters<typeof Validate>[0]) as Promise<SettingsValidationWire>
      },
      save(expectedRevision, candidate) {
        return Save(expectedRevision, candidate as Parameters<typeof Save>[1]) as Promise<SettingsSaveWire>
      },
      onChanged(listener) {
        return settingsChanged.onChanged(listener)
      },
    },
  }
}

function subscribe<T>(eventName: string, emit: (value: T) => void) {
  return EventsOn(eventName, (...values: unknown[]) => {
    emit(values[0] as T)
  })
}
