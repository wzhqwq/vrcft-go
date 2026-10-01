<script lang="ts">
  import {CircleAlert, Radio} from 'lucide-svelte'
  import type {PageId} from '../lib/components/layout/NavigationItems.svelte'
  import {Button} from '../lib/components/ui/index.js'
  import {AvatarDrivePanel, PluginOverviewPanel} from '../lib/patterns/index.js'
  import type {PluginsModule} from '../lib/modules/plugins/types.js'
  import type {RuntimeModule} from '../lib/modules/runtime/types.js'
  import {copy} from '../copy/zh-CN.js'

  let {runtime, plugins, onNavigate}: {runtime: RuntimeModule; plugins: PluginsModule; onNavigate: (page: PageId) => void} = $props()
  const osc = $derived(runtime.state.snapshot?.osc)
  const oscFault = $derived(osc?.state === 'error' || osc?.state === 'not_running')
  const oscLabel = $derived(osc?.state === 'manual' ? '手动目标' : osc?.state === 'discovered' ? '已连接' : osc?.state === 'discovering' ? '正在发现输出目标' : osc?.state === 'error' ? 'OSC 输出错误' : 'OSC 未运行')
</script>

<main class="page-grid min-w-0" aria-label="概览">
  <header class="min-w-0"><h1 class="text-xl font-bold">概览</h1></header>
  <div class="grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(0,1.3fr)_minmax(18rem,1fr)]">
    <AvatarDrivePanel state={runtime.state} {onNavigate} />
    <div class="grid min-w-0 content-start gap-4">
      <PluginOverviewPanel {plugins} failures={runtime.state.snapshot?.pluginFailures ?? []} {onNavigate} />
      <section class={`surface-card min-w-0 p-4 ${oscFault || runtime.state.status === 'stale' ? 'border-warning bg-warning/10' : ''}`} aria-labelledby="osc-overview-title">
        <div class="flex items-center gap-2">
          {#if oscFault}<CircleAlert size={18} class="text-warning" aria-hidden="true" />{:else}<Radio size={18} aria-hidden="true" />{/if}
          <h2 id="osc-overview-title" class="font-semibold">{copy.overview.oscOutput}</h2>
        </div>
        {#if runtime.state.status === 'stale'}<p class="mt-2 text-sm text-warning">数据可能已过期</p>{/if}
        <p class="mt-2 text-sm">{runtime.state.status === 'stale' ? `上次记录：${oscLabel}` : oscLabel}</p>
        {#if osc?.target}<p class="mt-1 break-all text-sm text-text-muted">{osc.target.host}:{osc.target.port}</p>{/if}
        {#if osc?.error}<p class="mt-1 break-words text-sm text-warning">{osc.error}</p>{/if}
        <div class="mt-3"><Button label={copy.overview.toSettings} tone="secondary" onclick={() => onNavigate('settings')} /></div>
      </section>
    </div>
  </div>
</main>
