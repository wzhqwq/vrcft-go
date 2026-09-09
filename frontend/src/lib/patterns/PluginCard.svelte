<script lang="ts">
  import {Badge, SwitchField} from '../components/ui/index.js';
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
</script>

<article class="surface-card grid min-w-0 gap-4" aria-labelledby={titleId}>
  <div class="flex min-w-0 flex-wrap items-start justify-between gap-3">
    <div class="min-w-0">
      <h2 class="min-w-0 break-words font-semibold text-text" id={titleId}>{name}</h2>
      <p class="min-w-0 break-all text-sm text-text-muted">{id}</p>
      {#if description}<p class="min-w-0 break-words text-sm text-text-muted">{description}</p>{/if}
    </div>
    <Badge label={active ? copy.state.active : copy.state.inactive} tone={active ? 'success' : 'neutral'} />
  </div>
  <div aria-label={copy.pluginCard.capabilities(name)}>
    <Inline gap="sm">{#each capabilities as capability (capability)}<Badge label={capability} />{/each}</Inline>
  </div>
  <DetailList items={[
    {label: copy.pluginCard.lifecycle, value: localizedState(copy.state.plugin, state)},
    {label: copy.pluginCard.frameRate, value: `${frameRate} FPS`},
    {label: copy.pluginCard.restarts, value: String(restartCount)},
  ]} />
  {#if onCommand}
    <SwitchField label={copy.pluginCard.enable(name)} description={loading ? copy.pluginCard.pending : undefined}
      disabled={loading} bind:checked={() => enabled, (value) => onCommand?.({pluginId: id, enabled: value})} />
  {:else}
    <Badge label={enabled ? copy.state.enabled : copy.state.disabled} />
  {/if}
  {#if problem}
    <ProblemBanner title={problem.title} detail={problem.detail} tone={problem.tone} diagnosticCode={problem.code} {onCopyDiagnostic} />
  {:else if error}
    <ProblemBanner title={copy.pluginCard.error} detail={error} tone="warning" />
  {/if}
</article>
