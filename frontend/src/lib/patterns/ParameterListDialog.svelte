<script lang="ts">
  import {Dialog} from '../components/ui/index.js'
  import type {RuntimePlanView} from '../modules/runtime/types.js'

  let {parameters}: {parameters: RuntimePlanView['parameters']} = $props()
</script>

<Dialog triggerLabel="查看参数列表" title="可驱动参数" description="仅列出软件支持的当前 Avatar 参数。" closeLabel="关闭">
  <ul class="grid max-h-[55dvh] min-w-0 gap-2 overflow-y-auto" aria-label="参数列表">
    {#each parameters as parameter (parameter.name)}
      <li class="flex min-w-0 items-center justify-between gap-3 rounded-lg border border-border px-3 py-2">
        <span class="min-w-0 break-all font-medium">{parameter.name}</span>
        <span class={`shrink-0 text-sm ${parameter.driven ? 'text-success' : 'text-text-muted'}`}>{parameter.driven ? '当前驱动' : '暂无有效输入'}</span>
      </li>
    {/each}
  </ul>
</Dialog>
