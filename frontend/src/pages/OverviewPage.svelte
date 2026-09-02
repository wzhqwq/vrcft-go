<script lang="ts">
  import {PageHeader, ResponsiveGrid, Stack} from '../lib/components/layout/index.js'
  import {AvatarSummary, DetailList, EmptyState, OSCSummary, ProblemBanner, StatusCard} from '../lib/patterns/index.js'
  import type {PluginsModule} from '../lib/modules/plugins/types.js'
  import type {RuntimeModule, RuntimeOscView} from '../lib/modules/runtime/types.js'

  type Props = {
    runtime: RuntimeModule;
    plugins: PluginsModule;
  }

  let {runtime, plugins}: Props = $props()

  function copyAvatarId(id: string) {
    void navigator.clipboard?.writeText(id)
  }

  function oscDiscoveryLabel(osc: RuntimeOscView): string {
    switch (osc.state) {
      case 'discovered': return '自动发现'
      case 'manual': return '手动目标'
      case 'discovering': return '正在发现'
      case 'not_running': return '未启动'
      case 'error': return '发现出错'
    }
  }

  function pluginSummary(): string {
    const summary = plugins.state.summary
    return `${summary.total} 个插件 · ${summary.active} 个活跃 · ${summary.problem} 个问题`
  }
</script>

<main class="page-grid min-w-0" aria-label="概览">
  <PageHeader title="概览" />

  {#if runtime.state.status === 'loading'}
    <StatusCard title="运行状态" loading loadingLabel="正在读取运行状态" />
  {:else if runtime.state.snapshot === null}
    {#if runtime.state.problem}
      <ProblemBanner
        title={runtime.state.problem.title}
        detail={runtime.state.problem.detail}
        tone={runtime.state.problem.tone}
        diagnosticCode={runtime.state.problem.code}
      />
    {/if}
    <EmptyState title="暂无可显示的运行状态" description="应用完成初始化后，当前 Avatar 和 OSC 输出会显示在这里。" />
  {:else}
    <Stack gap="lg">
      {#if runtime.state.status === 'stale' && runtime.state.problem}
        <ProblemBanner
          title="数据可能已过期"
          detail={runtime.state.problem.detail}
          tone="warning"
          diagnosticCode={runtime.state.problem.code}
        />
      {:else if runtime.state.status === 'problem' && runtime.state.problem}
        <ProblemBanner
          title={runtime.state.problem.title}
          detail={runtime.state.problem.detail}
          tone={runtime.state.problem.tone}
          diagnosticCode={runtime.state.problem.code}
        />
      {/if}

      <ResponsiveGrid>
        <AvatarSummary
          name={runtime.state.snapshot.avatar.name}
          id={runtime.state.snapshot.avatar.id}
          onCopyId={copyAvatarId}
        />
        {#if runtime.state.snapshot.osc}
          <section class="surface-card grid min-w-0 gap-3" aria-label="OSC 发现状态">
            <p class="text-sm text-text-muted">发现方式</p>
            <p class="font-semibold text-text">{oscDiscoveryLabel(runtime.state.snapshot.osc)}</p>
            <OSCSummary
              state={runtime.state.snapshot.osc.state}
              host={runtime.state.snapshot.osc.target?.host}
              port={runtime.state.snapshot.osc.target?.port}
              error={runtime.state.snapshot.osc.error}
            />
          </section>
        {:else}
          <StatusCard title="OSC 输出" detail="尚未提供 OSC 输出状态。" tone="warning" />
        {/if}
      </ResponsiveGrid>

      <ResponsiveGrid>
        {#if runtime.state.snapshot.plan}
          <section class="surface-card grid min-w-0 gap-3" aria-labelledby="plan-summary-title">
            <h2 class="font-semibold text-text" id="plan-summary-title">Avatar 计划</h2>
            <DetailList items={[
              {label: '计划状态', value: runtime.state.snapshot.plan.status},
              {label: '来源', value: runtime.state.snapshot.plan.source},
              {label: '代数', value: String(runtime.state.snapshot.plan.generation)},
              {label: '配置状态', value: runtime.state.snapshot.plan.configId || '尚未配置'},
            ]} />
          </section>
        {:else}
          <StatusCard title="Avatar 计划" detail="尚未生成可用计划。" tone="warning" />
        {/if}

        <StatusCard title="插件概览" detail={pluginSummary()} tone={plugins.state.summary.problem > 0 ? 'warning' : 'success'} />
      </ResponsiveGrid>

      {#if runtime.state.snapshot.pluginFailures.length > 0}
        <section class="grid min-w-0 gap-3" aria-labelledby="plugin-failures-title">
          <h2 class="font-semibold text-text" id="plugin-failures-title">插件操作问题</h2>
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
