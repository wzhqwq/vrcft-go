<script lang="ts">
  import {CircleHelp} from 'lucide-svelte'
  import {onDestroy, tick} from 'svelte'
  import {Dialog} from '../components/ui/index.js'
  import {copy, getLocale} from '../../copy/index.svelte.js'
  import type {PluginView} from '../modules/plugins/types.js'

  let {name, troubleshooting, fallbackFocus}: {name: string; troubleshooting: NonNullable<PluginView['troubleshooting']>; fallbackFocus?: () => HTMLElement | null} = $props()
  let open = $state(false)
  const steps = $derived(troubleshooting[getLocale()] ?? troubleshooting['en-US'])

  onDestroy(() => {
    if (open) void tick().then(() => fallbackFocus?.()?.focus())
  })
</script>

<Dialog
  bind:open
  triggerLabel={copy.pluginCard.troubleshootFor(name)}
  title={`${name} · ${copy.pluginCard.troubleshoot}`}
  triggerClass="focus-ring inline-flex size-8 shrink-0 items-center justify-center rounded-lg border border-warning/50 bg-warning/10 text-warning transition-colors hover:bg-warning/20"
>
  {#snippet trigger()}<CircleHelp size={18} aria-hidden="true" />{/snippet}
  <ol class="grid min-w-0 list-decimal gap-3 pl-6 text-sm text-text">
    {#each steps as step, index (index)}
      <li class="min-w-0 break-words pl-1">{step}</li>
    {/each}
  </ol>
</Dialog>
