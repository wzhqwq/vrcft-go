<script lang="ts">
  import {Badge, Button, Spinner} from '../components/ui/index.js';

  export interface PluginCardCommand {
    pluginId: string;
    enabled: boolean;
  }

  type Props = {
    id: string;
    name: string;
    description?: string;
    enabled: boolean;
    loading?: boolean;
    error?: string;
    onCommand?: (command: PluginCardCommand) => void;
  };

  let {id, name, description, enabled, loading = false, error, onCommand}: Props = $props();
  let actionLabel = $derived(enabled ? '停用插件' : '启用插件');
</script>

<article class="surface-card grid min-w-0 gap-3" aria-labelledby={`plugin-${id}`}>
  <div class="flex min-w-0 items-start justify-between gap-3">
    <div class="min-w-0">
      <h3 class="min-w-0 break-words font-semibold text-text" id={`plugin-${id}`}>{name}</h3>
      {#if description}<p class="min-w-0 break-words text-sm text-text-muted">{description}</p>{/if}
    </div>
    <Badge label={enabled ? '已启用' : '已停用'} tone={enabled ? 'success' : 'neutral'} />
  </div>
  {#if loading}
    <Spinner label="正在更新插件" />
  {:else}
    <Button label={actionLabel} tone="secondary" onclick={() => onCommand?.({pluginId: id, enabled: !enabled})} />
  {/if}
  {#if error}<p class="min-w-0 break-words text-sm text-danger" role="alert">{error}</p>{/if}
</article>
