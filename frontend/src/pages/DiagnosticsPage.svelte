<script lang="ts">
  import {Button} from '../lib/components/ui/index.js'
  import {PageHeader, ResponsiveGrid, Stack} from '../lib/components/layout/index.js'
  import {DetailList, EmptyState, ProblemBanner, StatusCard} from '../lib/patterns/index.js'
  import type {PluginsModule} from '../lib/modules/plugins/types.js'
  import type {RuntimeModule} from '../lib/modules/runtime/types.js'
  import type {SettingsModule} from '../lib/modules/settings/types.js'

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

  function statusLabel(status: ModuleStatus): string {
    switch (status) {
      case 'loading': return '正在加载'
      case 'ready': return '已就绪'
      case 'stale': return '数据可能已过期'
      case 'problem': return '启动失败'
    }
  }

  function metadata(revision: number | null, updatedAt: string | null) {
    return [
      {label: '修订', value: revision === null ? '尚无修订' : `修订 ${revision}`},
      {label: '更新时间', value: updatedAt ?? '尚未更新'},
    ]
  }

  function safeText(value: string | undefined, fallback = '未提供'): string {
    if (!value) return fallback
    const normalized = value.replace(/[\u0000-\u001f\u007f]/g, ' ').trim()
    return normalized.length <= maxDisplayLength ? normalized : `${normalized.slice(0, maxDisplayLength - 1)}…`
  }

  function problemTitle(problem: {title: string} | null): string | null {
    return problem === null ? null : safeText(problem.title)
  }

  function target(): string {
    const osc = runtime.state.snapshot?.osc
    return osc?.target ? `${safeText(osc.target.host)}:${osc.target.port}` : '未设置输出目标'
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

<main class="page-grid min-w-0" aria-label="诊断">
  <PageHeader title="Project Status" description="三个独立模块的当前可见状态。" />

  <Stack gap="lg">
    <section class="grid min-w-0 gap-3" aria-labelledby="module-status-title">
      <div class="flex min-w-0 flex-wrap items-center justify-between gap-3">
        <h2 class="text-lg font-semibold text-text" id="module-status-title">模块状态</h2>
        <Button label="复制诊断信息" tone="secondary" onclick={copyDiagnostics} />
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
              <span class="status-pill">{statusLabel(module.state.status)}</span>
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
      <StatusCard title="Runtime 状态" loading loadingLabel="正在读取 Runtime 状态" />
    {:else if runtime.state.snapshot === null}
      <EmptyState title="Runtime 尚无数据" description="Runtime 启动或恢复连接后，会显示应用和输出状态。" />
    {:else}
      {@const snapshot = runtime.state.snapshot}
      <ResponsiveGrid>
        <section class="surface-card grid min-w-0 gap-3" aria-labelledby="application-status-title">
          <h2 class="font-semibold text-text" id="application-status-title">应用</h2>
          <DetailList items={[
            {label: '生命周期', value: safeText(snapshot.lifecycle, '未提供')},
            {label: '平台支持', value: snapshot.platformSupported ? '受支持' : '不受支持'},
            {label: '应用阶段', value: safeText(snapshot.phase)},
          ]} />
        </section>

        {#if snapshot.plan}
          <section class="surface-card grid min-w-0 gap-3" aria-labelledby="plan-status-title">
            <h2 class="font-semibold text-text" id="plan-status-title">Avatar 计划</h2>
            <DetailList items={[
              {label: '计划状态', value: safeText(snapshot.plan.status)},
              {label: '来源', value: safeText(snapshot.plan.source)},
              {label: '配置', value: safeText(snapshot.plan.configId, '尚未配置')},
              {label: '代数', value: String(snapshot.plan.generation)},
            ]} />
          </section>
        {:else}
          <EmptyState title="尚未生成 Avatar 计划" description="收到 Avatar 变更后，会显示当前计划状态。" />
        {/if}

        {#if snapshot.osc}
          <section class="surface-card grid min-w-0 gap-3" aria-labelledby="osc-status-title">
            <h2 class="font-semibold text-text" id="osc-status-title">OSC 输出</h2>
            <DetailList items={[
              {label: '模式', value: safeText(snapshot.osc.state)},
              {label: '目标', value: target()},
              {label: '错误', value: snapshot.osc.error ? '有 OSC 错误' : '无'},
            ]} />
          </section>
        {:else}
          <EmptyState title="尚未提供 OSC 状态" description="OSC 启动后会显示当前输出目标。" />
        {/if}
      </ResponsiveGrid>

      {#if !snapshot.platformSupported}
        <ProblemBanner title="当前平台暂不受支持" detail="Runtime 仍会保留可用的状态信息。" tone="warning" diagnosticCode="unsupported_platform" />
      {/if}

      {#if snapshot.pluginFailures.length > 0}
        <section class="grid min-w-0 gap-3" aria-labelledby="plugin-failures-title">
          <h2 class="font-semibold text-text" id="plugin-failures-title">插件控制失败</h2>
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
      <EmptyState title="Plugins 启动失败" description="请检查插件服务后重试；其他模块仍可继续使用。" />
    {:else if plugins.state.snapshot?.plugins.length === 0}
      <EmptyState title="没有已发现的插件" description="发现或安装插件后，会在这里显示插件诊断。" />
    {/if}

    {#if settings.state.server === null && settings.state.status !== 'loading'}
      <EmptyState title="Settings 尚无数据" description="设置模块恢复连接后，会显示其修订和状态。" />
    {/if}
  </Stack>
</main>
