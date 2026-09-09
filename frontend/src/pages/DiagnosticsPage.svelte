<script lang="ts">
  import {copy} from '../copy/zh-CN.js'
  import {copyText} from '../lib/presentation/clipboard.js'
  import {Button} from '../lib/components/ui/index.js'
  import {PageHeader, ResponsiveGrid, Stack} from '../lib/components/layout/index.js'
  import {DetailList, EmptyState, ProblemBanner, StatusCard} from '../lib/patterns/index.js'
  import type {PluginsModule} from '../lib/modules/plugins/types.js'
  import type {RuntimeModule} from '../lib/modules/runtime/types.js'
  import type {SettingsModule} from '../lib/modules/settings/types.js'

  import {phasePresentation, localizedState} from '../lib/presentation/status.js'

  type Props = {
    runtime: RuntimeModule
    plugins: PluginsModule
    settings: SettingsModule
  }

  type ModuleName = 'Runtime' | 'Plugins' | 'Settings'
  type ModuleStatus = 'loading' | 'ready' | 'stale' | 'problem'

  const maxDiagnosticLength = 2048
  const maxDisplayLength = 160

  let {runtime, plugins, settings}: Props = $props()

  function metadata(revision: number | null, updatedAt: string | null) {
    return [
      {label: copy.text.revision, value: revision === null ? copy.text.noRevision : copy.format.revision(revision)},
      {label: copy.text.updatedAt, value: updatedAt ?? copy.text.notUpdated},
    ]
  }

  function safeText(value: string | undefined, fallback: string = copy.text.notProvided): string {
    if (!value) return fallback
    const normalized = value.replace(/[\u0000-\u001f\u007f]/g, ' ').trim()
    return normalized.length <= maxDisplayLength ? normalized : `${normalized.slice(0, maxDisplayLength - 1)}…`
  }

  function problemTitle(problem: {title: string} | null): string | null {
    return problem === null ? null : safeText(problem.title)
  }

  function target(): string {
    const osc = runtime.state.snapshot?.osc
    return osc?.target ? `${safeText(osc.target.host)}:${osc.target.port}` : copy.text.noTarget
  }

  function safeSummary(): string {
    const lines = [
      `Runtime: ${runtime.state.status}`,
      `Plugins: ${plugins.state.status}`,
      `Settings: ${settings.state.status}`,
      `Runtime revision: ${runtime.state.revision ?? 'none'}`,
      `Plugins revision: ${plugins.state.revision ?? 'none'}`,
      `Settings revision: ${settings.state.revision ?? 'none'}`,
    ]
    const osc = runtime.state.snapshot?.osc
    if (osc) lines.push(`OSC: ${osc.state} ${osc.target ? target() : 'none'}`)
    return lines.join('\n').slice(0, maxDiagnosticLength)
  }

  function copyDiagnostics() {
    const text = safeSummary()
    void navigator.clipboard?.writeText(text).catch(() => undefined)
  }
</script>

<main class="page-grid min-w-0" aria-label={copy.navigation.diagnostics}>
  <PageHeader title="Project Status" description={copy.text.diagnosticsDescription} />

  <Stack gap="lg">
    <section class="grid min-w-0 gap-3" aria-labelledby="module-status-title">
      <div class="flex min-w-0 flex-wrap items-center justify-between gap-3">
        <h2 class="text-lg font-semibold text-text" id="module-status-title">{copy.text.moduleStatus}</h2>
        <Button label={copy.text.copyDiagnostics} tone="secondary" onclick={copyDiagnostics} />
      </div>
      <ResponsiveGrid columns={3}>
        {#each [
          {name: 'Runtime' as ModuleName, state: runtime.state},
          {name: 'Plugins' as ModuleName, state: plugins.state},
          {name: 'Settings' as ModuleName, state: settings.state},
        ] as module (module.name)}
          <article class="surface-card grid min-w-0 gap-3" aria-label={module.name}>
            <div class="flex min-w-0 flex-wrap items-center justify-between gap-2">
              <h3 class="font-semibold text-text">{module.name}</h3>
              <span class="status-pill">{copy.state[module.state.status]}</span>
            </div>
            <DetailList items={metadata(module.state.revision, module.state.updatedAt)} />
            {#if problemTitle(module.state.problem)}
              <p class="min-w-0 break-words text-sm text-warning">{problemTitle(module.state.problem)}</p>
            {/if}
          </article>
        {/each}
      </ResponsiveGrid>
    </section>

    {#if runtime.state.status === 'loading'}
      <StatusCard title={copy.text.runtimeStatus} label={copy.state.loading} tone="neutral" loading loadingLabel={copy.text.readRuntime} />
    {:else if runtime.state.snapshot === null}
      <EmptyState title={copy.text.noRuntime} description={copy.text.noRuntimeDescription} />
    {:else}
      {@const snapshot = runtime.state.snapshot}
      <ResponsiveGrid>
        <section class="surface-card grid min-w-0 gap-3" aria-labelledby="application-status-title">
          <h2 class="font-semibold text-text" id="application-status-title">{copy.text.application}</h2>
          <DetailList items={[
            {label: copy.text.lifecycle, value: snapshot.lifecycle ? localizedState(copy.state.phase, snapshot.lifecycle) : copy.text.notProvided},
            {label: copy.text.platformSupport, value: snapshot.platformSupported ? copy.text.supported : copy.text.unsupported},
            {label: copy.text.phase, value: phasePresentation(snapshot.phase).label},
          ]} />
        </section>

        {#if snapshot.plan}
          <section class="surface-card grid min-w-0 gap-3" aria-labelledby="plan-status-title">
            <h2 class="font-semibold text-text" id="plan-status-title">{copy.text.avatarPlan}</h2>
            <DetailList items={[
              {label: copy.text.planStatus, value: localizedState(copy.state.plan, snapshot.plan.status)},
              {label: copy.text.source, value: safeText(snapshot.plan.source)},
              {label: copy.text.config, value: safeText(snapshot.plan.configId, copy.text.unconfigured)},
              {label: copy.text.generation, value: String(snapshot.plan.generation)},
            ]} />
            {#if snapshot.planError}
              <p class="min-w-0 break-words text-sm text-warning">{copy.text.planNeedsAttention}</p>
            {/if}
          </section>
        {:else}
          <EmptyState title={copy.text.noAvatarPlan} description={copy.text.noAvatarPlanDescription} />
        {/if}

        {#if snapshot.osc}
          <section class="surface-card grid min-w-0 gap-3" aria-labelledby="osc-status-title">
            <h2 class="font-semibold text-text" id="osc-status-title">{copy.text.oscOutput}</h2>
            <DetailList items={[
              {label: copy.text.mode, value: copy.state.osc[snapshot.osc.state].discovery},
              {label: copy.text.target, value: target()},
              {label: copy.text.error, value: snapshot.osc.error ? copy.text.oscError : copy.text.none},
            ]} />
          </section>
        {:else}
          <EmptyState title={copy.text.noOsc} description={copy.text.noOscDescription} />
        {/if}
      </ResponsiveGrid>

      {#if !snapshot.platformSupported}
        <ProblemBanner title={copy.text.unsupportedTitle} detail={copy.text.retainedRuntime} tone="warning" diagnosticCode="unsupported_platform" onCopyDiagnostic={copyText} />
      {/if}

      {#if snapshot.pluginFailures.length > 0}
        <section class="grid min-w-0 gap-3" aria-labelledby="plugin-failures-title">
          <h2 class="font-semibold text-text" id="plugin-failures-title">{copy.text.pluginControlFailed}</h2>
          {#each snapshot.pluginFailures as failure (failure.pluginId + failure.operation)}
            <article class="surface-card grid min-w-0 gap-2">
              <h3 class="min-w-0 break-words font-semibold text-text">{safeText(failure.pluginId)} · {safeText(failure.operation)}</h3>
              <p class="min-w-0 break-words text-sm text-text-muted">{safeText(failure.message)}</p>
            </article>
          {/each}
        </section>
      {/if}
    {/if}

    {#if plugins.state.status === 'problem' && plugins.state.snapshot === null}
      <EmptyState title={copy.text.pluginsStartupFailed} description={copy.text.pluginsStartupFailedDescription} />
    {:else if plugins.state.snapshot?.plugins.length === 0}
      <EmptyState title={copy.text.noPlugins} description={copy.text.pluginsDiagnosticsDescription} />
    {/if}

    {#if settings.state.server === null && settings.state.status !== 'loading'}
      <EmptyState title={copy.text.settingsNoData} description={copy.text.settingsNoDataDescription} />
    {/if}
  </Stack>
</main>
