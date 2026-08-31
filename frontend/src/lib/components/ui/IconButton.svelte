<script lang="ts">
  import type {Snippet} from 'svelte';

  type Props = {
    label: string;
    disabled?: boolean;
    children?: Snippet;
  };

  let {label, disabled = false, children}: Props = $props();
</script>

<button
  type="button"
  class="focus-ring inline-flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-surface-raised text-text transition-colors hover:bg-surface disabled:cursor-not-allowed disabled:opacity-60"
  aria-label={label}
  {disabled}
>
  {#if children}
    {@render children()}
  {:else}
    <span aria-hidden="true">•</span>
  {/if}
</button>
