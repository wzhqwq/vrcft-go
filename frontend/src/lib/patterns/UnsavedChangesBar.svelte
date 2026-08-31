<script lang="ts">
  import {Button} from '../components/ui/index.js';

  type Props = {
    message?: string;
    saving?: boolean;
    onSave: () => void;
    onDiscard: () => void;
  };

  let {message = '你有尚未保存的更改。', saving = false, onSave, onDiscard}: Props = $props();
</script>

<aside class="sticky bottom-0 z-10 flex min-w-0 flex-wrap items-center justify-between gap-3 border-t border-border bg-surface/95 px-4 py-3 backdrop-blur" role="region" aria-label="未保存的更改">
  <p class="min-w-0 break-words text-sm text-text">{message}</p>
  <div class="flex min-w-0 flex-wrap gap-2">
    <Button label="放弃更改" tone="secondary" disabled={saving} onclick={onDiscard} />
    <Button label="保存更改" loading={saving} loadingLabel="正在保存更改" onclick={onSave} />
  </div>
</aside>
