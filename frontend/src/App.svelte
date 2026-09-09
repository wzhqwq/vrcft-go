<script lang="ts">
  import {onMount} from 'svelte'

  import {copy} from './copy/zh-CN.js'
  import {AppShell, type NavigationItem, type PageId} from './lib/components/layout/index.js'
  import {Button, Dialog} from './lib/components/ui/index.js'
  import {createPluginsModule} from './lib/modules/plugins/index.js'
  import {createRuntimeModule} from './lib/modules/runtime/index.js'
  import {createSettingsModule} from './lib/modules/settings/index.js'
  import type {SettingsModule} from './lib/modules/settings/types.js'
  import {productionPorts} from './lib/wails/production.js'
  import type {WailsPorts} from './lib/wails/ports.js'
  import OverviewPage from './pages/OverviewPage.svelte'
  import PluginsPage from './pages/PluginsPage.svelte'
  import SettingsPage from './pages/SettingsPage.svelte'
  import DiagnosticsPage from './pages/DiagnosticsPage.svelte'

  type Props = {ports?: WailsPorts}
  type SettingsPageHandle = {canLeave(): boolean}

  let {ports = productionPorts()}: Props = $props()
  const appPorts: WailsPorts = (() => ports)()
  const runtime = createRuntimeModule(appPorts.runtime)
  const plugins = createPluginsModule(appPorts.plugins)
  const settings: SettingsModule = createSettingsModule(appPorts.settings)
  const navigation: NavigationItem[] = [
    {id: 'overview', label: copy.navigation.overview},
    {id: 'plugins', label: copy.navigation.plugins},
    {id: 'settings', label: copy.navigation.settings},
    {id: 'diagnostics', label: copy.navigation.diagnostics},
  ]

  let activePage = $state<PageId>('overview')
  let pendingPage = $state<PageId | null>(null)
  let confirmationOpen = $state(false)
  let settingsPage = $state<SettingsPageHandle | null>(null)

  onMount(() => {
    void Promise.all([startIndependently(runtime), startIndependently(plugins), startIndependently(settings)])
    return () => {
      runtime.dispose()
      plugins.dispose()
      settings.dispose()
    }
  })

  $effect(() => {
    if (!settings.state.dirty) return
    const protect = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', protect)
    return () => window.removeEventListener('beforeunload', protect)
  })

  function navigate(page: PageId) {
    if (page === activePage) return
    if (activePage === 'settings' && settingsPage !== null && !settingsPage.canLeave()) {
      pendingPage = page
      confirmationOpen = true
      return
    }
    activePage = page
  }

  function confirmNavigation() {
    const page = pendingPage
    pendingPage = null
    confirmationOpen = false
    if (page !== null) activePage = page
  }

  function startIndependently(module: {start(): Promise<void>}) {
    try {
      return Promise.resolve(module.start()).catch(() => undefined)
    } catch {
      return Promise.resolve()
    }
  }
</script>

{#snippet content()}
  {#if activePage === 'overview'}
    <OverviewPage {runtime} {plugins} />
  {:else if activePage === 'plugins'}
    <PluginsPage {plugins} />
  {:else if activePage === 'settings'}
    <SettingsPage {settings} bind:this={settingsPage} />
  {:else}
    <DiagnosticsPage {runtime} {plugins} {settings} />
  {/if}
{/snippet}

<AppShell {navigation} {activePage} onNavigate={navigate} {content} />

<Dialog
  triggerLabel={copy.text.leaveTrigger}
  title={copy.text.leaveTitle}
  description={copy.text.leaveDescription}
  closeLabel={copy.actions.cancel}
  bind:open={confirmationOpen}
  showTrigger={false}
>
  <Button label={copy.text.discard} tone="danger" onclick={confirmNavigation} />
</Dialog>
