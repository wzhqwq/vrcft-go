<script lang="ts">
  import { onMount } from "svelte"
  import { CircleAlert } from "lucide-svelte"
  import { copy } from "../copy/zh-CN.js"
  import { copyText } from "../lib/presentation/clipboard.js"
  import { diagnosticText, diagnosticLogPath } from "../lib/presentation/diagnostics.js"
  import { localTime } from "../lib/presentation/time.js"
  import type { DiagnosticEntryWire } from "../lib/wails/types.js"
  import { Button } from "../lib/components/ui/index.js"
  import { PageHeader, ResponsiveGrid, Stack } from "../lib/components/layout/index.js"
  import { DetailList, EmptyState, ProblemBanner, StatusCard } from "../lib/patterns/index.js"
  import type { PluginsModule } from "../lib/modules/plugins/types.js"
  import type { RuntimeModule } from "../lib/modules/runtime/types.js"
  import type { SettingsModule } from "../lib/modules/settings/types.js"

  import { phasePresentation, localizedState } from "../lib/presentation/status.js"

  type Props = {
    runtime: RuntimeModule
    plugins: PluginsModule
    settings: SettingsModule
  }

  type ModuleName = "Runtime" | "Plugins" | "Settings"
  const maxDiagnosticLength = 16384

  let { runtime, plugins, settings }: Props = $props()
  let level = $state("ALL")
  let copyStatus = $state("")
  let selectedModule = $state<ModuleName | null>(null)
  let filteredLogs = $derived(
    (runtime.diagnostics?.snapshot?.entries ?? [])
      .slice(-200)
      .filter(entry => level === "ALL" || entry.level === level),
  )

  let modules = $derived([
    { name: "Runtime" as ModuleName, state: runtime.state },
    { name: "Plugins" as ModuleName, state: plugins.state },
    { name: "Settings" as ModuleName, state: settings.state },
  ])
  let selectedProblem = $derived(
    modules.find(module => module.name === selectedModule)?.state.problem ?? null,
  )

  onMount(() => {
    void runtime.refreshDiagnostics?.()
    const timer = setInterval(() => {
      if (!runtime.diagnostics?.loading) void runtime.refreshDiagnostics?.()
    }, 3000)
    return () => clearInterval(timer)
  })

  function metadata(revision: number | null, updatedAt: string | null) {
    return [
      {
        label: copy.text.revision,
        value: revision === null ? copy.text.noRevision : copy.format.revision(revision),
      },
      { label: copy.text.updatedAt, value: localTime(updatedAt) },
    ]
  }

  function safeText(value: string | undefined, fallback: string = copy.text.notProvided): string {
    if (!value) return fallback
    return diagnosticText(value)
  }

  function problemTitle(problem: { title: string } | null): string | null {
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
      `Runtime revision: ${runtime.state.revision ?? "none"}`,
      `Plugins revision: ${plugins.state.revision ?? "none"}`,
      `Settings revision: ${settings.state.revision ?? "none"}`,
    ]
    const osc = runtime.state.snapshot?.osc
    if (osc) lines.push(`OSC: ${osc.state} ${osc.target ? target() : "none"}`)
    for (const [name, state] of [
      ["Runtime", runtime.state],
      ["Plugins", plugins.state],
      ["Settings", settings.state],
    ] as const) {
      lines.push(`${name} updated: ${localTime(state.updatedAt)}`)
      if (state.problem)
        lines.push(
          `${name} error [${safeText(state.problem.code)}]: ${safeText(state.problem.detail)}`,
        )
    }
    const snapshot = runtime.state.snapshot
    if (snapshot?.runtimeError) lines.push(`Runtime error: ${safeText(snapshot.runtimeError)}`)
    if (snapshot?.planError) lines.push(`Plan error: ${safeText(snapshot.planError)}`)
    if (osc?.error) lines.push(`OSC error: ${safeText(osc.error)}`)
    const diagnostic = runtime.diagnostics
    if (diagnostic?.snapshot?.failure)
      lines.push(`Startup: ${logLine(diagnostic.snapshot.failure)}`)
    if (diagnostic?.snapshot?.logPath)
      lines.push(`Log file: ${diagnosticLogPath(diagnostic.snapshot.logPath)}`)
    if (diagnostic?.snapshot?.diskError)
      lines.push(`Disk error: ${safeText(diagnostic.snapshot.diskError)}`)
    if (diagnostic?.error) lines.push(safeText(diagnostic.error))
    for (const failure of snapshot?.pluginFailures.slice(0, 20) ?? [])
      lines.push(
        `Plugin ${safeText(failure.pluginId)} ${safeText(failure.operation)}: ${safeText(failure.message)}`,
      )
    lines.push(...filteredLogs.slice(-20).map(logLine))
    return lines.join("\n").slice(0, maxDiagnosticLength)
  }

  function logLine(entry: DiagnosticEntryWire) {
    return `${localTime(entry.time)} ${safeText(entry.level)} ${safeText(entry.component)}/${safeText(entry.stage)} [${safeText(entry.id)}] ${safeText(entry.message)}`
  }

  function copyDiagnostics() {
    void copyDiagnosticText(safeSummary())
  }

  async function copyDiagnosticText(text: string) {
    try {
      if (!navigator.clipboard) throw new Error("clipboard unavailable")
      await navigator.clipboard.writeText(text)
      copyStatus = "已复制"
    } catch {
      copyStatus = "复制失败，请重试"
    }
  }
</script>

<main class="page-grid min-w-0" aria-label={copy.navigation.diagnostics}>
  <PageHeader title={copy.navigation.diagnostics} />
  {#if copyStatus}
    <p role="status" class="text-sm text-text-muted">{copyStatus}</p>
  {/if}

  <Stack gap="md">
    <section class="surface-card grid gap-2" aria-labelledby="recent-logs-title">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h2 id="recent-logs-title" class="font-semibold">最近日志</h2>
        <div class="flex flex-wrap items-center gap-2">
          <label class="flex items-center gap-2 text-sm">
            日志级别
            <select class="form-control focus-ring w-auto" bind:value={level}>
              <option value="ALL">全部</option>
              {#each ["DEBUG", "INFO", "WARN", "ERROR"] as value}
                <option {value}>
                  {value}
                </option>
              {/each}
            </select>
          </label>
          <Button
            label="刷新日志"
            tone="secondary"
            disabled={runtime.diagnostics?.loading ?? false}
            onclick={() => void runtime.refreshDiagnostics?.()}
          />
          <Button
            label="复制日志"
            tone="secondary"
            onclick={() =>
              void copyDiagnosticText(
                filteredLogs.map(logLine).join("\n").slice(0, maxDiagnosticLength),
              )}
          />
        </div>
      </div>
      {#if runtime.diagnostics?.error}
        <p role="status" class="break-words text-sm text-warning">
          {safeText(runtime.diagnostics.error)}
        </p>
      {/if}
      {#if runtime.diagnostics?.snapshot?.diskError}
        <p class="break-words text-sm text-warning">
          磁盘日志不可用：{safeText(runtime.diagnostics.snapshot.diskError)}
        </p>
      {/if}
      {#if runtime.diagnostics?.snapshot?.logPath}
        <p class="break-words text-xs text-text-muted">
          日志位置：{diagnosticLogPath(runtime.diagnostics.snapshot.logPath)}
        </p>
      {/if}
      <div class="max-h-72 min-w-0 overflow-y-auto" role="region" aria-label="最近日志内容">
        {#each filteredLogs as entry}
          <article class="grid gap-1 border-b border-border py-2 last:border-b-0">
            <p class="break-words text-xs text-text-muted">
              {localTime(entry.time)} ·
              <span
                class:text-danger={entry.level === "ERROR"}
                class:text-warning={entry.level === "WARN"}
              >
                {safeText(entry.level)}
              </span>
              {` · ${safeText(entry.component)}/${safeText(entry.stage)} · ${safeText(entry.id)}`}
            </p>
            <p class="break-words text-sm">{safeText(entry.message)}</p>
          </article>
        {:else}
          <p class="py-3 text-sm text-text-muted">
            {runtime.diagnostics?.loading ? "正在读取日志…" : "暂无匹配日志"}
          </p>
        {/each}
      </div>
    </section>

    <section class="grid min-w-0 gap-3" aria-labelledby="module-status-title">
      <div class="flex min-w-0 flex-wrap items-center justify-between gap-3">
        <h2 class="text-lg font-semibold text-text" id="module-status-title">
          {copy.text.moduleStatus}
        </h2>
        <Button label={copy.text.copyDiagnostics} tone="secondary" onclick={copyDiagnostics} />
      </div>
      <div class="flex gap-3">
        {#each modules as module (module.name)}
          <div class="surface-card grid min-w-0 gap-3">
            <div class="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1 text-sm">
              <h3 class="font-semibold text-text">{module.name}</h3>
              <span class="status-pill">{copy.state[module.state.status]}</span>
              {#if module.state.problem}
                <button
                  type="button"
                  class="focus-ring inline-flex size-7 shrink-0 items-center justify-center rounded-full border border-warning text-warning hover:bg-warning/10 aria-pressed:bg-warning/20"
                  aria-label={`查看 ${module.name} 报错`}
                  aria-pressed={selectedModule === module.name}
                  aria-controls="module-problem"
                  onclick={() => (selectedModule = module.name)}
                >
                  <CircleAlert size={16} aria-hidden="true" />
                </button>
              {/if}
            </div>
            <div class="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1 text-sm">
              {#each metadata(module.state.revision, module.state.updatedAt) as item}
                <span class="text-text-muted" title={item.label}>{item.value}</span>
              {/each}
            </div>
          </div>
        {/each}
      </div>
      {#if selectedProblem}
        <div id="module-problem" aria-live="polite">
          <div class="surface-card grid min-w-0 gap-2">
            <p class="min-w-0 break-words text-sm text-warning">
              {selectedModule}：{problemTitle(selectedProblem)}
            </p>
            <p class="min-w-0 break-words text-xs text-text-muted">
              错误代码：{safeText(selectedProblem.code)}
            </p>
            <p class="min-w-0 break-words text-sm text-text-muted">
              {safeText(selectedProblem.detail)}
            </p>
          </div>
        </div>
      {/if}
    </section>

    {#if runtime.diagnostics?.snapshot?.failure}
      {@const failure = runtime.diagnostics.snapshot.failure}
      <section class="surface-card grid gap-2 border-danger" aria-labelledby="startup-error-title">
        <h2 id="startup-error-title" class="font-semibold text-danger">启动错误</h2>
        <p class="break-words text-sm text-text-muted">
          {localTime(failure.time)} · {safeText(failure.component)}/{safeText(failure.stage)} · {safeText(
            failure.id,
          )}
        </p>
        <p class="break-words text-sm">{safeText(failure.message)}</p>
      </section>
    {/if}

    {#if runtime.state.status === "loading"}
      <StatusCard
        title={copy.text.runtimeStatus}
        label={copy.state.loading}
        tone="neutral"
        loading
        loadingLabel={copy.text.readRuntime}
      />
    {:else if runtime.state.snapshot === null}
      <EmptyState title={copy.text.noRuntime} description={copy.text.noRuntimeDescription} />
    {:else}
      {@const snapshot = runtime.state.snapshot}
      <ResponsiveGrid class="items-start">
        <section class="surface-card grid min-w-0 gap-3" aria-labelledby="application-status-title">
          <h2 class="font-semibold text-text" id="application-status-title">
            {copy.text.application}
          </h2>
          <DetailList
            items={[
              {
                label: copy.text.lifecycle,
                value: snapshot.lifecycle
                  ? localizedState(copy.state.phase, snapshot.lifecycle)
                  : copy.text.notProvided,
              },
              {
                label: copy.text.platformSupport,
                value: snapshot.platformSupported ? copy.text.supported : copy.text.unsupported,
              },
              { label: copy.text.phase, value: phasePresentation(snapshot.phase).label },
            ]}
          />
        </section>

        {#if snapshot.plan}
          <section class="surface-card grid min-w-0 gap-3" aria-labelledby="plan-status-title">
            <h2 class="font-semibold text-text" id="plan-status-title">{copy.text.avatarPlan}</h2>
            <DetailList
              items={[
                {
                  label: copy.text.planStatus,
                  value: localizedState(copy.state.plan, snapshot.plan.status),
                },
                { label: copy.text.source, value: safeText(snapshot.plan.source) },
                {
                  label: copy.text.config,
                  value: safeText(snapshot.plan.configId, copy.text.unconfigured),
                },
                { label: copy.text.generation, value: String(snapshot.plan.generation) },
              ]}
            />
            {#if snapshot.planError}
              <p class="min-w-0 break-words text-sm text-warning">{safeText(snapshot.planError)}</p>
            {/if}
          </section>
        {:else}
          <EmptyState
            title={copy.text.noAvatarPlan}
            description={copy.text.noAvatarPlanDescription}
          />
        {/if}

        {#if snapshot.osc}
          <section class="surface-card grid min-w-0 gap-3" aria-labelledby="osc-status-title">
            <h2 class="font-semibold text-text" id="osc-status-title">{copy.text.oscOutput}</h2>
            <DetailList
              items={[
                { label: copy.text.mode, value: copy.state.osc[snapshot.osc.state].discovery },
                { label: copy.text.target, value: target() },
                {
                  label: copy.text.error,
                  value: snapshot.osc.error ? safeText(snapshot.osc.error) : copy.text.none,
                },
              ]}
            />
          </section>
        {:else}
          <EmptyState title={copy.text.noOsc} description={copy.text.noOscDescription} />
        {/if}
      </ResponsiveGrid>

      {#if snapshot.runtimeError}
        <p class="surface-card break-words text-sm text-warning">
          {safeText(snapshot.runtimeError)}
        </p>
      {/if}

      {#if !snapshot.platformSupported}
        <ProblemBanner
          title={copy.text.unsupportedTitle}
          detail={copy.text.retainedRuntime}
          tone="warning"
          diagnosticCode="unsupported_platform"
          onCopyDiagnostic={copyText}
        />
      {/if}

      {#if snapshot.pluginFailures.length > 0}
        <section class="grid min-w-0 gap-3" aria-labelledby="plugin-failures-title">
          <h2 class="font-semibold text-text" id="plugin-failures-title">
            {copy.text.pluginControlFailed}
          </h2>
          {#each snapshot.pluginFailures as failure (failure.pluginId + failure.operation)}
            <article class="surface-card grid min-w-0 gap-2">
              <h3 class="min-w-0 break-words font-semibold text-text">
                {safeText(failure.pluginId)} · {safeText(failure.operation)}
              </h3>
              <p class="min-w-0 break-words text-sm text-text-muted">{safeText(failure.message)}</p>
            </article>
          {/each}
        </section>
      {/if}
    {/if}

    {#if plugins.state.status === "problem" && plugins.state.snapshot === null}
      <EmptyState
        title={copy.text.pluginsStartupFailed}
        description={copy.text.pluginsStartupFailedDescription}
      />
    {:else if plugins.state.snapshot?.plugins.length === 0}
      <EmptyState
        title={copy.text.noPlugins}
        description={copy.text.pluginsDiagnosticsDescription}
      />
    {/if}

    {#if settings.state.server === null && settings.state.status !== "loading"}
      <EmptyState
        title={copy.text.settingsNoData}
        description={copy.text.settingsNoDataDescription}
      />
    {/if}
  </Stack>
</main>
