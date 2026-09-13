<script lang="ts">
  import {PageHeader, ResponsiveGrid, Stack} from '../lib/components/layout/index.js'
  import {AvatarSummary, DetailList, EmptyState, OSCSummary, ProblemBanner, StatusCard, PluginCard} from '../lib/patterns/index.js'
  import type {PluginsModule} from '../lib/modules/plugins/types.js'
  import type {RuntimeModule} from '../lib/modules/runtime/types.js'

  import {copy} from '../copy/zh-CN.js'
  import {copyText} from '../lib/presentation/clipboard.js'
  import {phasePresentation, localizedState} from '../lib/presentation/status.js'
  import {selectImportantPlugins} from '../lib/modules/plugins/selectors.js'

  type Props = {
    runtime: RuntimeModule;
    plugins: PluginsModule;
  }

  let {runtime, plugins}: Props = $props()

  const importantPlugins = $derived(selectImportantPlugins(plugins.state.snapshot?.plugins ?? []))

  function pluginSummary(): string {
    const summary = plugins.state.summary
    return copy.format.pluginSummary(summary.total, summary.active, summary.problem)
  }
</script>

<main class="page-grid min-w-0" aria-label={copy.navigation.overview}>
  <PageHeader title={copy.navigation.overview} />

  {#if runtime.state.status === 'loading'}
    <StatusCard title={copy.text.runStatus} label={copy.state.loading} tone="neutral" loading loadingLabel={copy.text.readRunStatus} />
  {:else if runtime.state.snapshot === null}
    {#if runtime.state.problem}
      <ProblemBanner
        title={runtime.state.problem.title}
        detail={runtime.state.problem.detail}
        tone={runtime.state.problem.tone}
        diagnosticCode={runtime.state.problem.code} onCopyDiagnostic={copyText}
      />
    {/if}
    <EmptyState title={copy.text.noRunStatus} description={copy.text.noRunDescription} />
  {:else}
    <Stack gap="lg">
      {#if runtime.state.status === 'stale' && runtime.state.problem}
        <ProblemBanner
          title={copy.state.stale}
          detail={runtime.state.problem.detail}
          tone="warning"
          diagnosticCode={runtime.state.problem.code} onCopyDiagnostic={copyText}
        />
      {:else if runtime.state.status === 'problem' && runtime.state.problem}
        <ProblemBanner
          title={runtime.state.problem.title}
          detail={runtime.state.problem.detail}
          tone={runtime.state.problem.tone}
          diagnosticCode={runtime.state.problem.code} onCopyDiagnostic={copyText}
        />
      {/if}

      {#if runtime.state.snapshot.avatar.id && runtime.state.snapshot.plan?.status === 'failed' && !runtime.state.snapshot.plan.configPath}
        <ProblemBanner
          title={copy.text.missingAvatarConfig}
          detail={copy.text.missingAvatarConfigDescription}
          tone="warning"
        />
      {/if}

      <StatusCard title={copy.text.phase} label={phasePresentation(runtime.state.snapshot.phase).label} tone={phasePresentation(runtime.state.snapshot.phase).tone} />
      <ResponsiveGrid>
        <AvatarSummary
          name={runtime.state.snapshot.avatar.name}
          id={runtime.state.snapshot.avatar.id}
          onCopyId={copyText}
        />
        {#if runtime.state.snapshot.osc}
          <OSCSummary
              state={runtime.state.snapshot.osc.state}
              host={runtime.state.snapshot.osc.target?.host}
              port={runtime.state.snapshot.osc.target?.port}
              error={runtime.state.snapshot.osc.error}
            />
        {:else}
          <StatusCard title={copy.text.oscOutput} detail={copy.text.oscAbsent} label={copy.state.absent} tone="warning" />
        {/if}
      </ResponsiveGrid>

      <ResponsiveGrid>
        {#if runtime.state.snapshot.plan}
          <section class="surface-card grid min-w-0 gap-3" aria-labelledby="plan-summary-title">
            <h2 class="font-semibold text-text" id="plan-summary-title">{copy.text.avatarPlan}</h2>
            <DetailList items={[
              {label: copy.text.planStatus, value: localizedState(copy.state.plan, runtime.state.snapshot.plan.status)},
              {label: copy.text.source, value: runtime.state.snapshot.plan.source},
              {label: copy.text.generation, value: String(runtime.state.snapshot.plan.generation)},
              {label: copy.text.configState, value: runtime.state.snapshot.plan.configId || copy.text.unconfigured},
            ]} />
          </section>
        {:else}
          <StatusCard title={copy.text.avatarPlan} detail={copy.text.noPlan} label={copy.state.absent} tone="warning" />
        {/if}

        <div class="grid min-w-0 gap-3">
          {#if plugins.state.status === 'loading'}
            <StatusCard title={copy.text.pluginOverview} label={copy.state.loading} tone="neutral" loading loadingLabel={copy.text.readPluginOverview} />
          {:else if plugins.state.snapshot === null}
            {#if plugins.state.problem}
              <ProblemBanner
                title={plugins.state.problem.title}
                detail={plugins.state.problem.detail}
                tone={plugins.state.problem.tone}
                diagnosticCode={plugins.state.problem.code} onCopyDiagnostic={copyText}
              />
            {/if}
            <EmptyState title={copy.text.noPluginOverview} description={copy.text.noPluginOverviewDescription} />
          {:else}
            {#if plugins.state.status === 'stale' && plugins.state.problem}
              <ProblemBanner
                title={copy.state.stale}
                detail={plugins.state.problem.detail}
                tone="warning"
                diagnosticCode={plugins.state.problem.code} onCopyDiagnostic={copyText}
              />
            {:else if plugins.state.status === 'problem' && plugins.state.problem}
              <ProblemBanner
                title={plugins.state.problem.title}
                detail={plugins.state.problem.detail}
                tone={plugins.state.problem.tone}
                diagnosticCode={plugins.state.problem.code} onCopyDiagnostic={copyText}
              />
            {/if}

            {#if plugins.state.snapshot.plugins.length === 0}
              <EmptyState title={copy.text.noPlugins} description={copy.text.noPluginsDescription} />
            {:else}
              <StatusCard title={copy.text.pluginOverview} label={plugins.state.summary.problem > 0 ? copy.state.attention : copy.state.normal} detail={pluginSummary()} tone={plugins.state.summary.problem > 0 ? 'warning' : 'success'} />
            {/if}
          {/if}
        </div>
      </ResponsiveGrid>

      {#if importantPlugins.length > 0}
        <section class="grid min-w-0 gap-3" aria-label={copy.pluginCard.important}>
          <h2 class="font-semibold text-text">{copy.pluginCard.important}</h2>
          <ResponsiveGrid>
            {#each importantPlugins as plugin (plugin.id)}
              <PluginCard {...plugin} loading={plugins.state.pendingIds.has(plugin.id)}
                error={plugin.lastError} problem={plugins.state.problems.get(plugin.id)}
                onCommand={({pluginId, enabled}) => { void plugins.setEnabled(pluginId, enabled) }}
                onCopyDiagnostic={copyText} />
            {/each}
          </ResponsiveGrid>
        </section>
      {/if}

      {#if runtime.state.snapshot.pluginFailures.length > 0}
        <section class="grid min-w-0 gap-3" aria-labelledby="plugin-failures-title">
          <h2 class="font-semibold text-text" id="plugin-failures-title">{copy.text.pluginOperations}</h2>
          {#each runtime.state.snapshot.pluginFailures as failure (failure.pluginId + failure.operation)}
            <ProblemBanner
              title={`${failure.pluginId}：${failure.operation}`}
              detail={failure.message}
              tone="warning"
            />
          {/each}
        </section>
      {/if}
    </Stack>
  {/if}
</main>
