<script lang="ts">
  import {copy} from '../../../copy/index.svelte.js';
  type Props = {label?: string};
  let {label}: Props = $props();
</script>

<span class="inline-flex min-w-0 items-center gap-2 text-text-muted" role="status">
  <span aria-hidden="true" class="size-4 animate-spin rounded-full border-2 border-current border-r-transparent"></span>
  <span>{label ?? copy.state.loading}</span>
</span>
