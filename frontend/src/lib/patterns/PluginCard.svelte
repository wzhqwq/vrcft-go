<script lang="ts">
  import {Switch} from 'bits-ui';
  import {Badge, Tooltip} from '../components/ui/index.js';
  import {Inline} from '../components/layout/index.js';
  import DetailList from './DetailList.svelte';
  import ProblemBanner from './ProblemBanner.svelte';
  import type {ProblemView} from '../presentation/problem.js';
  import {copy} from '../../copy/zh-CN.js';
  import {localizedState} from '../presentation/status.js';

  export interface PluginCardCommand {pluginId: string; enabled: boolean}
  type Props = {
    id: string; name: string; description?: string; enabled: boolean; active?: boolean;
    state?: string; capabilities?: readonly string[]; frameRate?: number; restartCount?: number;
    loading?: boolean; error?: string; problem?: ProblemView;
    onCommand?: (command: PluginCardCommand) => void;
    onCopyDiagnostic?: (text: string) => void;
  };
  let {id, name, description, enabled, active = false, state = '', capabilities = [], frameRate = 0,
    restartCount = 0, loading = false, error, problem, onCommand, onCopyDiagnostic}: Props = $props();
  const titleId = $props.id();
  const pendingId = `${titleId}-pending`;
  let details = $derived([
    {label: copy.pluginCard.lifecycle, value: localizedState(copy.state.plugin, state)},
    ...(active ? [{label: copy.pluginCard.frameRate, value: `${frameRate.toFixed(1)} FPS`}] : []),
    {label: copy.pluginCard.restarts, value: String(restartCount)},
  ]);
</script>

<article class="surface-card grid min-w-0 gap-4" aria-labelledby={titleId}>
  <header class="flex min-w-0 items-start justify-between gap-3">
    <div class="min-w-0">
      <h2 class="min-w-0 break-words font-semibold text-text" id={titleId}>{name}</h2>
      <p class="min-w-0 break-all text-sm text-text-muted">{id}</p>
      {#if description}<p class="min-w-0 break-words text-sm text-text-muted">{description}</p>{/if}
    </div>
    <div class="flex shrink-0 items-center gap-2">
      <Badge label={active ? copy.state.active : copy.state.inactive} tone={active ? 'success' : 'neutral'} />
      {#if onCommand}
        <Tooltip content={copy.pluginCard.enable(name)}>
          {#snippet trigger(props)}
            <Switch.Root
              {...props}
              bind:checked={() => enabled, (value) => onCommand?.({pluginId: id, enabled: value})}
              disabled={loading}
              aria-label={copy.pluginCard.enable(name)}
              aria-describedby={loading ? pendingId : undefined}
              class="focus-ring inline-flex h-6 w-11 shrink-0 items-center rounded-full border border-border bg-surface-raised p-0.5 transition-colors data-[state=checked]:border-accent data-[state=checked]:bg-accent disabled:cursor-not-allowed disabled:opacity-60"
            >
              <Switch.Thumb class="size-5 rounded-full bg-text transition-transform data-[state=checked]:translate-x-5" />
            </Switch.Root>
          {/snippet}
        </Tooltip>
        {#if loading}<span class="sr-only" id={pendingId}>{copy.pluginCard.pending}</span>{/if}
      {:else}
        <Badge label={enabled ? copy.state.enabled : copy.state.disabled} />
      {/if}
    </div>
  </header>
  <div aria-label={copy.pluginCard.capabilities(name)}>
    <Inline gap="sm">{#each capabilities as capability (capability)}<Badge label={capability} />{/each}</Inline>
  </div>
  <DetailList items={details} />
  {#if problem}
    <ProblemBanner title={problem.title} detail={problem.detail} tone={problem.tone} diagnosticCode={problem.code} {onCopyDiagnostic} />
  {:else if error}
    <ProblemBanner title={copy.pluginCard.error} detail={error} tone="warning" />
  {/if}
</article>
