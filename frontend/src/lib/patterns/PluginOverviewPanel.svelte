<script lang="ts">
  import {CircleAlert, Glasses, Power, ScanFace, Smile} from 'lucide-svelte'
  import {Button} from '../components/ui/index.js'
  import type {PageId} from '../components/layout/NavigationItems.svelte'
  import type {PluginsModule} from '../modules/plugins/types.js'
  import type {RuntimePluginFailureView} from '../modules/runtime/types.js'
  import {selectOverviewPlugins} from '../modules/plugins/selectors.js'
  import {copy} from '../../copy/zh-CN.js'

  let {plugins, failures = [], onNavigate}: {plugins: PluginsModule; failures?: readonly RuntimePluginFailureView[]; onNavigate: (page: PageId) => void} = $props()
  const items = $derived(selectOverviewPlugins(plugins.state.snapshot?.plugins ?? []))
  const trouble = $derived(failures.length > 0 || items.some((item) => ['crashed', 'unresponsive', 'incompatible'].includes(item.state)))
</script>

<section class={`surface-card min-w-0 p-4 ${trouble ? 'border-warning bg-warning/10' : ''}`} aria-labelledby="plugin-overview-title">
  <div class="flex items-center gap-2">
    {#if trouble}<CircleAlert size={18} class="text-warning" aria-hidden="true" />{/if}
    <h2 id="plugin-overview-title" class="font-semibold">{copy.overview.pluginInput}</h2>
    {#if plugins.state.status === 'stale'}<span class="status-pill ml-auto border-warning text-warning">{copy.overview.stale}</span>{/if}
  </div>
  {#if plugins.state.problem}<p class="mt-2 break-words text-sm text-warning">{plugins.state.problem.detail}</p>{/if}
  {#if items.length === 0}
    <p class="mt-3 text-sm text-text-muted">{plugins.state.status === 'loading' ? '正在读取插件…' : '未发现插件'}</p>
  {:else}
    <ul class="mt-3 grid min-w-0 gap-2">
      {#each items as item (item.id)}
        <li data-testid="overview-plugin-row" data-plugin-id={item.id} class="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-start gap-2 rounded-lg border border-border bg-surface p-3">
          <div class="min-w-0">
            <p class="truncate font-semibold" title={item.name}>{item.name}</p>
            <div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-text-muted">
              <span>{copy.state.plugin[item.state as keyof typeof copy.state.plugin] ?? item.state}</span>
              {#if item.active && item.state === 'running' && item.frameRate > 0}<span>{Math.round(item.frameRate)} FPS</span>{/if}
            </div>
            <div class="mt-2 flex items-center gap-2 text-text-muted">
              {#if item.capabilities.some((capability) => capability.toLowerCase() === 'eye')}<span title="Eye 输出能力" aria-label="Eye 输出能力"><Glasses size={17} aria-hidden="true" /></span>{/if}
              {#if item.capabilities.some((capability) => capability.toLowerCase() === 'expression')}<span title="Expression 输出能力" aria-label="Expression 输出能力"><ScanFace size={17} aria-hidden="true" /></span>{/if}
              {#if item.capabilities.some((capability) => capability.toLowerCase() === 'lip')}<span title="Lip 输出能力" aria-label="Lip 输出能力"><Smile size={17} aria-hidden="true" /></span>{/if}
            </div>
            {#if plugins.state.problems.get(item.id)}<p class="mt-2 break-words text-sm text-danger">{plugins.state.problems.get(item.id)?.detail}</p>{/if}
            {#if item.lastError}<p class="mt-2 break-words text-sm text-danger">{item.lastError}</p>{/if}
          </div>
          <button type="button" class="focus-ring rounded-lg border border-border p-2 disabled:opacity-50" aria-label={`${item.enabled ? '停用' : '启用'} ${item.name}`} title={`${item.enabled ? '停用' : '启用'} ${item.name}`} aria-pressed={item.enabled} disabled={plugins.state.pendingIds.has(item.id)} onclick={() => { void plugins.setEnabled(item.id, !item.enabled) }}><Power size={18} aria-hidden="true" /></button>
        </li>
      {/each}
    </ul>
  {/if}
  {#each failures.slice(0, 2) as failure (failure.pluginId + failure.operation)}
    <p class="mt-2 break-words rounded-lg border border-warning/40 bg-warning/10 p-2 text-sm text-warning">{failure.pluginId}：{failure.message}</p>
  {/each}
  {#if failures.length > 2}<p class="mt-1 text-sm text-warning">另有 {failures.length - 2} 项插件操作问题</p>{/if}
  <div class="mt-3"><Button label={copy.overview.allPlugins} tone="secondary" onclick={() => onNavigate('plugins')} /></div>
</section>
