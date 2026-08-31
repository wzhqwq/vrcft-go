<script lang="ts">
  import {Button, Spinner} from '../components/ui/index.js';

  type Props = {
    name: string;
    id: string;
    loading?: boolean;
    error?: string;
    onCopyId?: (id: string) => void;
  };

  let {name, id, loading = false, error, onCopyId}: Props = $props();
</script>

<section class="surface-card grid min-w-0 gap-3" aria-labelledby="avatar-summary-title">
  <div class="flex min-w-0 items-center justify-between gap-3">
    <h2 class="min-w-0 font-semibold text-text" id="avatar-summary-title">当前 Avatar</h2>
    <Button label="复制 Avatar ID" tone="secondary" onclick={() => onCopyId?.(id)} />
  </div>
  {#if loading}
    <Spinner label="正在读取 Avatar 信息" />
  {:else if error}
    <p class="text-sm text-danger" role="alert">{error}</p>
  {:else}
    <dl class="grid min-w-0 gap-2 text-sm">
      <div class="grid min-w-0 gap-1">
        <dt class="text-text-muted">名称</dt>
        <dd class="min-w-0 break-words text-text">{name || '未命名 Avatar'}</dd>
      </div>
      <div class="grid min-w-0 gap-1">
        <dt class="text-text-muted">Avatar ID</dt>
        <dd class="min-w-0 break-all text-text">{id}</dd>
      </div>
    </dl>
  {/if}
</section>
