<script lang="ts">
  import type {Snippet} from 'svelte';
  import type {HTMLButtonAttributes} from 'svelte/elements';

  type Props = Omit<HTMLButtonAttributes, 'children' | 'class' | 'disabled' | 'type'> & {
    label: string;
    type?: 'button' | 'submit' | 'reset';
    disabled?: boolean;
    children?: Snippet;
    class?: string;
  };

  let {label, type = 'button', disabled = false, children, class: className = '', ...rest}: Props = $props();
</script>

<button
  {...rest}
  {type}
  class={`focus-ring inline-flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-surface-raised text-text transition-colors hover:bg-surface disabled:cursor-not-allowed disabled:opacity-60 ${className}`}
  aria-label={label}
  {disabled}
>
  {#if children}
    {@render children()}
  {:else}
    <span aria-hidden="true">•</span>
  {/if}
</button>
