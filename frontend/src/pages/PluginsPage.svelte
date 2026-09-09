<script lang="ts">
  import {copy} from '../copy/zh-CN.js'
  import {copyText} from '../lib/presentation/clipboard.js'
  import {Button, TextField} from '../lib/components/ui/index.js'
  import {Inline, PageHeader, ResponsiveGrid, Stack} from '../lib/components/layout/index.js'
  import {PluginCard, EmptyState, ProblemBanner, StatusCard} from '../lib/patterns/index.js'
  import type {PluginFilter, PluginsModule} from '../lib/modules/plugins/types.js'

  type Props = {plugins: PluginsModule}

  const filters: Array<{value: PluginFilter; label: string}> = [
    {value: 'all', label: copy.text.filterAll},
    {value: 'enabled', label: copy.state.enabled},
    {value: 'disabled', label: copy.state.disabled},
    {value: 'problem', label: copy.text.filterProblems},
  ]

  let {plugins}: Props = $props()

  function selectFilter(filter: PluginFilter) {
    if (plugins.state.query.filter !== filter) plugins.setFilter(filter)
  }
</script>

<main class="page-grid min-w-0" aria-label={copy.navigation.plugins}>
  <PageHeader title={copy.navigation.plugins} description={copy.text.pluginsDescription} />

  {#if plugins.state.status === 'loading'}
    <StatusCard title={copy.text.pluginList} label={copy.state.loading} tone="neutral" loading loadingLabel={copy.text.readPluginList} />
  {:else}
    <Stack gap="lg">
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

      {#if plugins.state.snapshot === null}
        <EmptyState title={copy.text.noPluginData} description={copy.text.pluginReconnect} />
      {:else}
        <section class="surface-card grid min-w-0 gap-4" aria-label={copy.text.pluginFilters}>
          <TextField
            label={copy.text.pluginSearch}
            role="searchbox"
            value={plugins.state.query.query}
            placeholder={copy.text.pluginSearchPlaceholder}
            oninput={(event) => plugins.setQuery(event.currentTarget.value)}
          />
          <div aria-label={copy.text.pluginStateFilter}>
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
          <p class="text-sm text-text-muted">{copy.format.pluginCounts(plugins.state.filteredTotal, plugins.state.summary.enabled, plugins.state.summary.active, plugins.state.summary.problem)}</p>
        </section>

        {#if plugins.state.visiblePlugins.length === 0}
          <EmptyState title={copy.text.noMatchingPlugins} description={copy.text.adjustFilters} />
        {:else}
          <ResponsiveGrid columns={3}>
            {#each plugins.state.visiblePlugins as plugin (plugin.id)}
              <PluginCard {...plugin} loading={plugins.state.pendingIds.has(plugin.id)}
                error={plugin.lastError} problem={plugins.state.problems.get(plugin.id)}
                onCommand={({pluginId, enabled}) => { void plugins.setEnabled(pluginId, enabled) }}
                onCopyDiagnostic={copyText} />
            {/each}
          </ResponsiveGrid>
        {/if}

        {#if plugins.state.pageCount > 1}
          <nav class="flex min-w-0 flex-wrap items-center justify-between gap-3" aria-label={copy.text.pagination}>
            <Button
              label={copy.text.previousPage}
              tone="secondary"
              disabled={plugins.state.query.page <= 1}
              onclick={() => plugins.setPage(plugins.state.query.page - 1)}
            />
            <p class="text-sm text-text-muted">{copy.format.page(plugins.state.query.page, plugins.state.pageCount)}</p>
            <Button
              label={copy.text.nextPage}
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
