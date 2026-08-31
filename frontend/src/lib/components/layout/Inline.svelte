<script lang="ts">
  import type {Snippet} from 'svelte';

  type Align = 'start' | 'center' | 'end';
  type Gap = 'sm' | 'md' | 'lg';
  type Props = {
    align?: Align;
    gap?: Gap;
    children: Snippet;
    class?: string;
  };

  const alignClasses: Record<Align, string> = {
    start: 'items-start',
    center: 'items-center',
    end: 'items-end',
  };
  const gapClasses: Record<Gap, string> = {
    sm: 'gap-2',
    md: 'gap-4',
    lg: 'gap-6',
  };

  let {align = 'center', gap = 'md', children, class: className = ''}: Props = $props();
</script>

<div class={`flex min-w-0 flex-wrap ${alignClasses[align]} ${gapClasses[gap]} ${className}`}>{@render children()}</div>
