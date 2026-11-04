<script lang="ts">
  import {CircleAlert, Radio} from 'lucide-svelte'
  import type {PageId} from '../lib/components/layout/NavigationItems.svelte'
  import {Button} from '../lib/components/ui/index.js'
  import {AvatarDrivePanel, PluginOverviewPanel} from '../lib/patterns/index.js'
  import type {PluginsModule} from '../lib/modules/plugins/types.js'
  import type {RuntimeModule} from '../lib/modules/runtime/types.js'
  import {copy} from '../copy/index.svelte.js'

  let {runtime, plugins, onNavigate}: {runtime: RuntimeModule; plugins: PluginsModule; onNavigate: (page: PageId) => void} = $props()
  const osc = $derived(runtime.state.snapshot?.osc)
  const oscFault = $derived(osc?.state === 'error' || osc?.state === 'not_running')
  const oscLabel = $derived(osc?.state === 'manual' ? copy.text.overviewManualTarget : osc?.state === 'discovered' ? copy.text.overviewConnected : osc?.state === 'discovering' ? copy.text.overviewDiscovering : osc?.state === 'error' ? copy.state.osc.error.label : copy.state.osc.not_running.label)
</script>

<main class="page-grid min-w-0" aria-label={copy.navigation.overview}>
  <header class="min-w-0"><h1 class="text-xl font-bold">{copy.navigation.overview}</h1></header>
  <div class="grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(0,1.3fr)_minmax(18rem,1fr)]">
    <AvatarDrivePanel state={runtime.state} {onNavigate} />
    <div class="grid min-w-0 content-start gap-4">
      <PluginOverviewPanel {plugins} failures={runtime.state.snapshot?.pluginFailures ?? []} {onNavigate} />
      <section class={`surface-card min-w-0 p-4 ${oscFault || runtime.state.status === 'stale' ? 'border-warning bg-warning/10' : ''}`} aria-labelledby="osc-overview-title">
        <div class="flex items-center gap-2">
          {#if oscFault}<CircleAlert size={18} class="text-warning" aria-hidden="true" />{:else}<Radio size={18} aria-hidden="true" />{/if}
          <h2 id="osc-overview-title" class="font-semibold">{copy.overview.oscOutput}</h2>
        </div>
        {#if runtime.state.status === 'stale'}<p class="mt-2 text-sm text-warning">{copy.overview.stale}</p>{/if}
        <p class="mt-2 text-sm">{runtime.state.status === 'stale' ? copy.text.lastRecord(oscLabel) : oscLabel}</p>
        {#if osc?.target}<p class="mt-1 break-all text-sm text-text-muted">{osc.target.host}:{osc.target.port}</p>{/if}
        {#if osc?.error}<p class="mt-1 break-words text-sm text-warning">{osc.error}</p>{/if}
        <div class="mt-3"><Button label={copy.overview.toSettings} tone="secondary" onclick={() => onNavigate('settings')} /></div>
      </section>
    </div>
  </div>
</main>
