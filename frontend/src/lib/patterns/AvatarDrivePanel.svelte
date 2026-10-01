<script lang="ts">
  import {CircleAlert, CircleCheck, CircleHelp, Clock3} from 'lucide-svelte'
  import {Button} from '../components/ui/index.js'
  import type {PageId} from '../components/layout/NavigationItems.svelte'
  import type {RuntimeModuleState} from '../modules/runtime/types.js'
  import ParameterListDialog from './ParameterListDialog.svelte'
  import {copy} from '../../copy/zh-CN.js'

  let {state, onNavigate}: {state: RuntimeModuleState; onNavigate: (page: PageId) => void} = $props()
  const snapshot = $derived(state.snapshot)
  const plan = $derived(snapshot?.plan)
  const missingConfig = $derived(Boolean(snapshot?.avatar.id && plan?.status === 'failed' && !plan.configPath))
  const fault = $derived(Boolean(plan?.status === 'failed' || state.status === 'problem' || snapshot?.runtimeError))
  const pending = $derived(state.status === 'loading' || Boolean(snapshot?.avatar.id && plan?.status !== 'ready' && plan?.status !== 'failed'))
  const count = $derived(plan?.parameters.filter((item) => item.driven).length ?? 0)
</script>

<section class={`surface-card min-w-0 self-start p-5 ${fault ? 'border-danger bg-danger/10' : state.status === 'stale' ? 'border-warning bg-warning/10' : ''}`} aria-labelledby="avatar-drive-title">
  <div class="flex items-center gap-2">
    {#if fault}<CircleAlert size={20} aria-hidden="true" class="text-danger" />{:else if pending}<Clock3 size={20} aria-hidden="true" class="text-warning" />{:else if plan?.status === 'ready'}<CircleCheck size={20} aria-hidden="true" class="text-success" />{:else}<CircleHelp size={20} aria-hidden="true" />{/if}
    <h2 id="avatar-drive-title" class="font-semibold">{copy.overview.avatarDrive}</h2>
  </div>
  {#if state.status === 'stale'}<p class="mt-2 text-sm text-warning">{copy.overview.stale}</p>{/if}
  {#if snapshot?.avatar.id}
    <p class="mt-3 break-words text-xl font-bold">{snapshot.avatar.name || snapshot.avatar.id}</p>
    {#if plan?.status === 'ready'}
      {#if plan.parameters.length > 0}
        <p class="mt-3 text-lg font-semibold">{state.status === 'stale' ? '最近记录' : '当前驱动'} {count} / 可驱动 {plan.parameters.length}</p>
        <p class="mt-1 text-sm text-text-muted">依据近期有效插件输入</p>
        <div class="mt-4"><ParameterListDialog parameters={plan.parameters} stale={state.status === 'stale'} /></div>
      {:else}
        <p class="mt-3 text-text-muted">该 Avatar 没有请求软件支持的驱动参数。</p>
      {/if}
    {:else if plan?.status === 'failed'}
      <p class="mt-3 font-semibold text-danger">{missingConfig ? '未找到当前 Avatar 的配置' : '角色驱动计划失败'}</p>
      {#if snapshot.planError}<p class="mt-1 break-words text-sm text-text-muted">{snapshot.planError}</p>{/if}
      {#if missingConfig}<div class="mt-4"><Button label={copy.overview.toSettings} onclick={() => onNavigate('settings')} /></div>{/if}
    {:else}
      <p class="mt-3 text-text-muted">正在准备计划…</p>
    {/if}
  {:else if state.status === 'loading'}
    <p class="mt-3 text-text-muted">正在读取角色状态…</p>
  {:else}
    <p class="mt-3 text-text-muted">等待 Avatar…</p>
  {/if}
  {#if state.status === 'problem' && state.problem}
    <p class="mt-2 break-words text-sm text-danger">{state.problem.detail}</p>
  {/if}
  {#if snapshot?.runtimeError}
    <p class="mt-3 break-words rounded-lg border border-danger/40 bg-danger/10 p-3 text-sm text-danger">运行时异常：{snapshot.runtimeError}</p>
    <div class="mt-3"><Button label="前往诊断" tone="secondary" onclick={() => onNavigate('diagnostics')} /></div>
  {/if}
</section>
