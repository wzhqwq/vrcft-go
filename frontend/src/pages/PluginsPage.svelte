<script lang="ts">
  import {Badge, Button, SwitchField, TextField} from '../lib/components/ui/index.js'
  import {Inline, PageHeader, ResponsiveGrid, Stack} from '../lib/components/layout/index.js'
  import {DetailList, EmptyState, ProblemBanner, StatusCard} from '../lib/patterns/index.js'
  import type {PluginFilter, PluginsModule, PluginView} from '../lib/modules/plugins/types.js'

  type Props = {plugins: PluginsModule}

  const filters: Array<{value: PluginFilter; label: string}> = [
    {value: 'all', label: '全部'},
    {value: 'enabled', label: '已启用'},
    {value: 'disabled', label: '已停用'},
    {value: 'problem', label: '只看问题'},
  ]

  let {plugins}: Props = $props()

  function setEnabled(plugin: PluginView, enabled: boolean) {
    if (plugins.state.pendingIds.has(plugin.id)) return
    void plugins.setEnabled(plugin.id, enabled)
  }

  function selectFilter(filter: PluginFilter) {
    if (plugins.state.query.filter !== filter) plugins.setFilter(filter)
  }
</script>

<main class="page-grid min-w-0" aria-label="插件">
  <PageHeader title="插件" description="搜索、查看并立即管理本地插件。" />

  {#if plugins.state.status === 'loading'}
    <StatusCard title="插件列表" loading loadingLabel="正在读取插件列表" />
  {:else}
    <Stack gap="lg">
      {#if plugins.state.status === 'stale' && plugins.state.problem}
        <ProblemBanner
          title="数据可能已过期"
          detail={plugins.state.problem.detail}
          tone="warning"
          diagnosticCode={plugins.state.problem.code}
        />
      {:else if plugins.state.status === 'problem' && plugins.state.problem}
        <ProblemBanner
          title={plugins.state.problem.title}
          detail={plugins.state.problem.detail}
          tone={plugins.state.problem.tone}
          diagnosticCode={plugins.state.problem.code}
        />
      {/if}

      {#if plugins.state.snapshot === null}
        <EmptyState title="暂无可显示的插件" description="应用恢复连接后会显示插件列表。" />
      {:else}
        <section class="surface-card grid min-w-0 gap-4" aria-label="插件筛选">
          <TextField
            label="搜索插件"
            role="searchbox"
            value={plugins.state.query.query}
            placeholder="按名称或 ID 搜索"
            oninput={(event) => plugins.setQuery(event.currentTarget.value)}
          />
          <div aria-label="插件状态筛选">
            <Inline gap="sm">
              {#each filters as filter (filter.value)}
                <Button
                  label={filter.label}
                  tone={plugins.state.query.filter === filter.value ? 'primary' : 'secondary'}
                  aria-pressed={plugins.state.query.filter === filter.value}
                  onclick={() => selectFilter(filter.value)}
                />
              {/each}
            </Inline>
          </div>
          <p class="text-sm text-text-muted">共 {plugins.state.filteredTotal} 个插件 · 已启用 {plugins.state.summary.enabled} · 活跃 {plugins.state.summary.active} · 问题 {plugins.state.summary.problem}</p>
        </section>

        {#if plugins.state.visiblePlugins.length === 0}
          <EmptyState title="没有符合条件的插件" description="请调整搜索词或状态筛选条件。" />
        {:else}
          <ResponsiveGrid columns={3}>
            {#each plugins.state.visiblePlugins as plugin (plugin.id)}
              {@const pending = plugins.state.pendingIds.has(plugin.id)}
              {@const problem = plugins.state.problems.get(plugin.id)}
              <article class="surface-card grid min-w-0 gap-4" aria-labelledby={`plugin-${plugin.id}`}>
                <div class="flex min-w-0 flex-wrap items-start justify-between gap-3">
                  <div class="min-w-0">
                    <h2 class="min-w-0 break-words font-semibold text-text" id={`plugin-${plugin.id}`}>{plugin.name}</h2>
                    <p class="min-w-0 break-all text-sm text-text-muted">{plugin.id}</p>
                  </div>
                  <Badge label={plugin.active ? '活跃' : '未活跃'} tone={plugin.active ? 'success' : 'neutral'} />
                </div>

                <div aria-label={`${plugin.name} 能力`}>
                  <Inline gap="sm">
                    {#each plugin.capabilities as capability (capability)}
                      <Badge label={capability} />
                    {/each}
                  </Inline>
                </div>

                <DetailList items={[
                  {label: '生命周期', value: plugin.state},
                  {label: '帧率', value: `${plugin.frameRate} FPS`},
                  {label: '重启次数', value: String(plugin.restartCount)},
                ]} />

                <SwitchField
                  label={`启用 ${plugin.name}`}
                  description={pending ? '正在更新此插件。' : undefined}
                  disabled={pending}
                  bind:checked={() => plugin.enabled, (enabled) => setEnabled(plugin, enabled)}
                />

                {#if problem}
                  <ProblemBanner title={problem.title} detail={problem.detail} tone={problem.tone} diagnosticCode={problem.code} />
                {:else if plugin.lastError}
                  <ProblemBanner title="插件运行提示" detail={plugin.lastError} tone="warning" />
                {/if}
              </article>
            {/each}
          </ResponsiveGrid>
        {/if}

        {#if plugins.state.pageCount > 1}
          <nav class="flex min-w-0 flex-wrap items-center justify-between gap-3" aria-label="插件分页">
            <Button
              label="上一页"
              tone="secondary"
              disabled={plugins.state.query.page <= 1}
              onclick={() => plugins.setPage(plugins.state.query.page - 1)}
            />
            <p class="text-sm text-text-muted">第 {plugins.state.query.page} 页，共 {plugins.state.pageCount} 页</p>
            <Button
              label="下一页"
              tone="secondary"
              disabled={plugins.state.query.page >= plugins.state.pageCount}
              onclick={() => plugins.setPage(plugins.state.query.page + 1)}
            />
          </nav>
        {/if}
      {/if}
    </Stack>
  {/if}
</main>
