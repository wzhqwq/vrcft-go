<script lang="ts">
  import type {Snippet} from 'svelte';

  type Gap = 'sm' | 'md' | 'lg';
  type Props = {
    gap?: Gap;
    children: Snippet;
    class?: string;
  };

  const gapClasses: Record<Gap, string> = {
    sm: 'gap-2',
    md: 'gap-4',
    lg: 'gap-6',
  };

  let {gap = 'md', children, class: className = ''}: Props = $props();
</script>

<div class={`flex min-w-0 flex-col ${gapClasses[gap]} ${className}`}>{@render children()}</div>
