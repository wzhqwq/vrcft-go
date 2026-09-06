import type {Page} from '@playwright/test';

export const viewports = [
  {width: 640, height: 480},
  {width: 720, height: 600},
  {width: 1024, height: 768},
  {width: 1440, height: 900},
] as const;

export async function installWailsMocks(page: Page): Promise<void> {
  await page.addInitScript(() => {
    type Listener = (...values: unknown[]) => void;
    type Call = [string, ...unknown[]];

    const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;
    const calls: Call[] = [];
    const listeners = new Map<string, Set<Listener>>();
    const pendingPluginMutations = new Map<string, (value: unknown) => void>();
    let holdPluginID: string | null = null;
    let nextSaveConflict = false;
    let copiedText = '';

    const channel = {
      calibration: {enabled: true, neutral: 0, min: -1, max: 1, gain: 1, invert: false},
      tuning: {deadzone: 0, gain: 1, exponent: 1, clampEnabled: true, clampMin: -1, clampMax: 1},
      filter: {mode: 'ema', emaAlpha: 0.5, minCutoff: 1, beta: 0, derivativeCutoff: 1},
      dropout: {holdDurationMs: 10, decayDurationMs: 20, staleAfterMs: 30},
    };
    const state = {
      runtime: {
        revision: 1, updatedAt: '2026-09-01T00:00:00Z', phase: 'running', platformSupported: true,
        application: {
          lifecycle: 'started', avatarId: 'avtr_authoritative', avatarName: 'Authoritative Avatar',
          planGeneration: 7, planStatus: 'ready', planSource: 'VRChat', configPath: 'C:\\RAW_CONFIG_PATH_DO_NOT_LEAK',
          configId: 'avtr_authoritative', generationExhausted: false,
          osc: {running: true, connected: true, hasTarget: true, targetMode: 'auto', target: {host: '192.168.1.10', port: 9000}},
          pluginFailures: [], planError: 'RAW_PLAN_ERROR_DO_NOT_LEAK', runtimeError: 'RAW_RUNTIME_ERROR_DO_NOT_LEAK',
        },
      },
      plugins: {
        revision: 1, updatedAt: '2026-09-01T00:00:00Z', plugins: [
          {
            id: 'eye', name: 'Eye Tracker', description: 'Eye tracking input', version: '1.0.0', capabilities: ['eyes'],
            enabled: true, active: true, state: 'running', configRevision: 1, frameRate: 60,
            consecutiveFailures: 0, restartCount: 0, startedAt: null, lastHeartbeatAt: null, lastFrameAt: null, nextRestartAt: null,
          },
          {
            id: 'lip', name: 'Lip Tracker', description: 'Lip tracking input', version: '1.0.0', capabilities: ['lips'],
            enabled: true, active: true, state: 'running', configRevision: 1, frameRate: 60,
            consecutiveFailures: 0, restartCount: 0, startedAt: null, lastHeartbeatAt: null, lastFrameAt: null, nextRestartAt: null,
          },
        ],
      },
      settings: {
        revision: 1, fileRevision: 1, updatedAt: '2026-09-01T00:00:00Z',
        settings: {
          avatar: {oscRoot: 'C:\\VRChat\\OSC', fallbackPath: 'C:\\VRChat\\fallback.json'},
          plugins: {devRoots: ['C:\\RAW_PRIVATE_VALUE_DO_NOT_LEAK']},
          processing: {
            defaultChannel: channel,
            overrides: [{name: 'eye.left_gaze_x', channel}],
            activeStaleAfterMs: 1000,
            mutualExclusion: [['eye.left_gaze_x', 'eye.right_gaze_x']],
          },
          osc: {targetMode: 'auto', preferredService: 'VRChat-Client', manualHost: '127.0.0.1', manualPort: 9000},
        },
      },
    };

    const subscribe = (eventName: string, callback: Listener, maxCallbacks = -1) => {
      const wrapped: Listener = (...values) => {
        callback(...values);
        if (maxCallbacks === 1) listeners.get(eventName)?.delete(wrapped);
      };
      const eventListeners = listeners.get(eventName) ?? new Set<Listener>();
      eventListeners.add(wrapped);
      listeners.set(eventName, eventListeners);
      return () => eventListeners.delete(wrapped);
    };
    const emit = (eventName: string, value: unknown) => {
      for (const listener of [...(listeners.get(eventName) ?? [])]) listener(value);
    };
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: {writeText: async (text: string) => { copiedText = text; }},
    });

    const runtime = {
      EventsOn: (eventName: string, callback: Listener) => subscribe(eventName, callback),
      EventsOnMultiple: (eventName: string, callback: Listener, maxCallbacks: number) => subscribe(eventName, callback, maxCallbacks),
      EventsOff: (...eventNames: string[]) => { for (const eventName of eventNames) listeners.delete(eventName); },
      EventsOffAll: () => listeners.clear(),
      EventsOnce: (eventName: string, callback: Listener) => subscribe(eventName, callback, 1),
      EventsEmit: (eventName: string, ...values: unknown[]) => emit(eventName, values[0]),
    };

    window.go = {
      main: {
        RuntimeAPI: {
          GetStatus: async () => { calls.push(['RuntimeAPI.GetStatus']); return clone(state.runtime); },
        },
        PluginsAPI: {
          GetConfig: async (pluginID: string) => { calls.push(['PluginsAPI.GetConfig', pluginID]); return {revision: 1, updatedAt: state.plugins.updatedAt, pluginId: pluginID, configRevision: 1, data: ''}; },
          List: async () => { calls.push(['PluginsAPI.List']); return clone(state.plugins); },
          SetEnabled: async (pluginID: string, enabled: boolean) => {
            calls.push(['PluginsAPI.SetEnabled', pluginID, enabled]);
            state.plugins.revision += 1;
            state.plugins.updatedAt = '2026-09-01T00:00:01Z';
            const plugin = state.plugins.plugins.find((item) => item.id === pluginID);
            if (plugin) plugin.enabled = enabled;
            const response = {revision: state.plugins.revision, updatedAt: state.plugins.updatedAt, pluginId: pluginID};
            if (holdPluginID === pluginID) return new Promise((resolve) => pendingPluginMutations.set(pluginID, resolve));
            return clone(response);
          },
          UpdateConfig: async (pluginID: string, revision: number, data: string) => { calls.push(['PluginsAPI.UpdateConfig', pluginID, revision, data]); return {revision, updatedAt: state.plugins.updatedAt, pluginId: pluginID}; },
        },
        SettingsAPI: {
          Get: async () => { calls.push(['SettingsAPI.Get']); return clone(state.settings); },
          Validate: async (candidate: unknown) => {
            calls.push(['SettingsAPI.Validate', clone(candidate)]);
            return {revision: state.settings.revision, updatedAt: state.settings.updatedAt, settings: clone(candidate)};
          },
          Save: async (expectedRevision: number, candidate: unknown) => {
            calls.push(['SettingsAPI.Save', expectedRevision, clone(candidate)]);
            if (nextSaveConflict) {
              nextSaveConflict = false;
              state.settings.revision = 2;
              state.settings.fileRevision = 2;
              state.settings.updatedAt = '2026-09-01T00:00:02Z';
              state.settings.settings = {
                ...state.settings.settings,
                avatar: {...state.settings.settings.avatar, oscRoot: 'C:\\Authoritative revision 2'},
              };
              return {
                revision: state.settings.revision, fileRevision: state.settings.fileRevision, updatedAt: state.settings.updatedAt,
                settings: clone(state.settings.settings), restartRequired: false,
                problem: {code: 'conflict', message: 'settings changed elsewhere', currentRevision: state.settings.revision},
              };
            }
            state.settings.revision += 1;
            state.settings.fileRevision += 1;
            state.settings.updatedAt = '2026-09-01T00:00:02Z';
            state.settings.settings = clone(candidate);
            return {...clone(state.settings), restartRequired: true};
          },
        },
      },
    };
    window.runtime = runtime;
    (window as Window & {__vrcftAcceptance: unknown}).__vrcftAcceptance = {
      calls: () => clone(calls),
      get copiedText() { return copiedText; },
      emit,
      holdPluginMutation: (pluginID: string) => { holdPluginID = pluginID; },
      resolvePluginMutation: (pluginID: string) => {
        const resolve = pendingPluginMutations.get(pluginID);
        pendingPluginMutations.delete(pluginID);
        holdPluginID = null;
        resolve?.({revision: state.plugins.revision, updatedAt: state.plugins.updatedAt, pluginId: pluginID});
      },
      conflictNextSave: () => { nextSaveConflict = true; },
    };
  });
}

export async function calls(page: Page): Promise<unknown[][]> {
  return page.evaluate(() => (window as unknown as {__vrcftAcceptance: {calls(): unknown[][]}}).__vrcftAcceptance.calls());
}

export async function emit(page: Page, eventName: 'vrcft:v1:runtime-status' | 'vrcft:v1:plugins-changed' | 'vrcft:v1:settings-changed'): Promise<void> {
  await page.evaluate((event) => (window as unknown as {__vrcftAcceptance: {emit(name: string, value: unknown): void}}).__vrcftAcceptance.emit(event, {revision: 99}), eventName);
}
