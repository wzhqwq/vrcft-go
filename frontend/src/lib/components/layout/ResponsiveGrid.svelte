<script lang="ts">
  import type {Snippet} from 'svelte';

  type ColumnCount = 1 | 2 | 3;
  type Props = {
    columns?: ColumnCount;
    children: Snippet;
    class?: string;
  };

  const columnClasses: Record<ColumnCount, string> = {
    1: 'grid-cols-1',
    2: 'grid-cols-1 @[32rem]:grid-cols-2',
    3: 'grid-cols-1 @[32rem]:grid-cols-2 @[52rem]:grid-cols-3',
  };

  let {columns = 2, children, class: className = ''}: Props = $props();
</script>

<div class={`@container grid min-w-0 gap-4 ${columnClasses[columns]} ${className}`}>{@render children()}</div>
