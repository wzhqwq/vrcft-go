<script lang="ts">
  import {CircleAlert, Glasses, Power, ScanFace, Smile} from 'lucide-svelte'
  import {Button} from '../components/ui/index.js'
  import PluginTroubleshootingDialog from './PluginTroubleshootingDialog.svelte'
  import type {PageId} from '../components/layout/NavigationItems.svelte'
  import type {PluginsModule} from '../modules/plugins/types.js'
  import type {RuntimePluginFailureView} from '../modules/runtime/types.js'
  import {pluginDeviceUnavailable, selectOverviewPlugins} from '../modules/plugins/selectors.js'
  import {copy} from '../../copy/index.svelte.js'

  let {plugins, failures = [], onNavigate}: {plugins: PluginsModule; failures?: readonly RuntimePluginFailureView[]; onNavigate: (page: PageId) => void} = $props()
  const items = $derived(selectOverviewPlugins(plugins.state.snapshot?.plugins ?? []))
  const trouble = $derived(failures.length > 0 || items.some((item) => ['crashed', 'unresponsive', 'incompatible'].includes(item.state) || pluginDeviceUnavailable(item)))
</script>

<section class={`surface-card min-w-0 p-4 ${trouble ? 'border-warning bg-warning/10' : ''}`} aria-labelledby="plugin-overview-title">
  <div class="flex items-center gap-2">
    {#if trouble}<CircleAlert size={18} class="text-warning" aria-hidden="true" />{/if}
    <h2 id="plugin-overview-title" class="font-semibold">{copy.overview.pluginInput}</h2>
    {#if plugins.state.status === 'stale'}<span class="status-pill ml-auto border-warning text-warning">{copy.overview.stale}</span>{/if}
  </div>
  {#if plugins.state.problem}<p class="mt-2 break-words text-sm text-warning">{plugins.state.problem.detail}</p>{/if}
  {#if items.length === 0}
    <p class="mt-3 text-sm text-text-muted">{plugins.state.status === 'loading' ? copy.text.readingPlugins : copy.text.noPlugins}</p>
  {:else}
    <ul class="mt-3 grid min-w-0 gap-2">
      {#each items as item (item.id)}
        <li id={`overview-plugin-${item.id}`} data-testid="overview-plugin-row" data-plugin-id={item.id} tabindex="-1" class={`grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-start gap-2 rounded-lg border bg-surface p-3 ${pluginDeviceUnavailable(item) ? 'border-warning' : 'border-border'}`}>
          <div class="min-w-0">
            <p class="truncate font-semibold" title={item.name}>{item.name}</p>
            <div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-text-muted">
              <span>{copy.state.plugin[item.state as keyof typeof copy.state.plugin] ?? item.state}</span>
			  {#if item.enabled && item.state === 'running' && item.deviceStatus}<span class={pluginDeviceUnavailable(item) ? 'text-warning' : ''}>{copy.state.device[item.deviceStatus.state as keyof typeof copy.state.device] ?? item.deviceStatus.state}</span>{/if}
              {#if item.active && item.state === 'running' && item.frameRate > 0}<span>{Math.round(item.frameRate)} FPS</span>{/if}
            </div>
            <div class="mt-2 flex items-center gap-2 text-text-muted">
              {#if item.capabilities.some((capability) => capability.toLowerCase() === 'eye')}<span title={copy.text.eyeCapability} aria-label={copy.text.eyeCapability}><Glasses size={17} aria-hidden="true" /></span>{/if}
              {#if item.capabilities.some((capability) => capability.toLowerCase() === 'expression')}<span title={copy.text.expressionCapability} aria-label={copy.text.expressionCapability}><ScanFace size={17} aria-hidden="true" /></span>{/if}
              {#if item.capabilities.some((capability) => capability.toLowerCase() === 'lip')}<span title={copy.text.lipCapability} aria-label={copy.text.lipCapability}><Smile size={17} aria-hidden="true" /></span>{/if}
            </div>
            {#if plugins.state.problems.get(item.id)}<p class="mt-2 break-words text-sm text-danger">{plugins.state.problems.get(item.id)?.detail}</p>{/if}
            {#if item.lastError}<p class="mt-2 break-words text-sm text-danger">{item.lastError}</p>{/if}
			{#if pluginDeviceUnavailable(item)}
			  <div class="mt-2 flex min-w-0 items-start justify-between gap-2 text-warning">
			    <p class="min-w-0 break-words text-sm">{item.deviceStatus?.message || copy.pluginCard.noTrackingData}</p>
			    {#if item.troubleshooting}<PluginTroubleshootingDialog name={item.name} troubleshooting={item.troubleshooting} fallbackFocus={() => document.getElementById(`overview-plugin-${item.id}`)} />{/if}
			  </div>
			{/if}
          </div>
          <button type="button" class="focus-ring rounded-lg border border-border p-2 disabled:opacity-50" aria-label={item.enabled ? copy.text.disablePlugin(item.name) : copy.pluginCard.enable(item.name)} title={item.enabled ? copy.text.disablePlugin(item.name) : copy.pluginCard.enable(item.name)} aria-pressed={item.enabled} disabled={plugins.state.pendingIds.has(item.id)} onclick={() => { void plugins.setEnabled(item.id, !item.enabled) }}><Power size={18} aria-hidden="true" /></button>
        </li>
      {/each}
    </ul>
  {/if}
  {#each failures.slice(0, 2) as failure (failure.pluginId + failure.operation)}
    <p class="mt-2 break-words rounded-lg border border-warning/40 bg-warning/10 p-2 text-sm text-warning">{failure.pluginId}：{failure.message}</p>
  {/each}
  {#if failures.length > 2}<p class="mt-1 text-sm text-warning">{copy.text.morePluginProblems(failures.length - 2)}</p>{/if}
  <div class="mt-3"><Button label={copy.overview.allPlugins} tone="secondary" onclick={() => onNavigate('plugins')} /></div>
</section>
