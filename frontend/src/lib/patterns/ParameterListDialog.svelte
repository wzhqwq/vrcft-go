<script lang="ts">
  import {Dialog} from '../components/ui/index.js'
  import {copy} from '../../copy/index.svelte.js'
  import type {RuntimePlanView} from '../modules/runtime/types.js'

  let {parameters, stale = false}: {parameters: RuntimePlanView['parameters']; stale?: boolean} = $props()
</script>

<Dialog triggerLabel={copy.overview.parameterList} title={copy.text.drivableParameters} description={copy.text.parameterListDescription} closeLabel={copy.text.close}>
  {#if stale}<p class="rounded-lg border border-warning bg-warning/10 px-3 py-2 text-sm text-warning">{copy.overview.stale}</p>{/if}
  <ul class="grid max-h-[55dvh] min-w-0 gap-2 overflow-y-auto" aria-label={copy.text.parameterList}>
    {#each parameters as parameter (parameter.name)}
      <li class="flex min-w-0 items-center justify-between gap-3 rounded-lg border border-border px-3 py-2">
        <span class="min-w-0 break-all font-medium">{parameter.name}</span>
        <span class={`shrink-0 text-sm ${parameter.driven ? 'text-success' : 'text-text-muted'}`}>{stale ? parameter.driven ? copy.text.lastHadInput : copy.text.lastNoInput : parameter.driven ? copy.text.currentlyDriven : copy.text.noValidInput}</span>
      </li>
    {/each}
  </ul>
</Dialog>
