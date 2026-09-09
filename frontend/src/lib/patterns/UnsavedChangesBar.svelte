<script lang="ts">
  import {copy} from '../../copy/zh-CN.js'
  import {Button} from '../components/ui/index.js';

  type Props = {
    message?: string;
    saving?: boolean;
    canSave?: boolean;
    onSave: () => void;
    onDiscard: () => void;
  };

  let {message = copy.text.unsavedDescription, saving = false, canSave = true, onSave, onDiscard}: Props = $props();
</script>

<aside class="sticky bottom-0 z-10 flex min-w-0 flex-wrap items-center justify-between gap-3 border-t border-border bg-surface/95 px-4 py-3 backdrop-blur" role="region" aria-label={copy.text.unsaved}>
  <p class="min-w-0 break-words text-sm text-text">{message}</p>
  <div class="flex min-w-0 flex-wrap gap-2">
    <Button label={copy.text.discard} tone="secondary" disabled={saving} onclick={onDiscard} />
    <Button label={copy.text.saveChanges} disabled={!canSave} loading={saving} loadingLabel={copy.text.savingChanges} onclick={onSave} />
  </div>
</aside>
