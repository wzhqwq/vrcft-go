<script module lang="ts">
  export type PageId = 'overview' | 'tracking' | 'plugins' | 'settings';

  export interface NavigationItem {
    id: PageId;
    label: string;
  }
</script>

<script lang="ts">
  import {Button} from '../ui/index.js';

  type Orientation = 'horizontal' | 'vertical';

  type Props = {
    items: NavigationItem[];
    activePage: PageId;
    onNavigate: (page: PageId) => void;
    orientation: Orientation;
    class?: string;
  };

  const orientationClasses: Record<Orientation, string> = {
    horizontal: 'flex min-w-max items-center gap-2 px-4 py-2',
    vertical: 'flex min-w-0 flex-col gap-2 p-4',
  };

  let {items, activePage, onNavigate, orientation, class: className = ''}: Props = $props();
</script>

<ul class={`${orientationClasses[orientation]} ${className}`}>
  {#each items as item (item.id)}
    <li class={orientation === 'vertical' ? 'min-w-0' : 'shrink-0'}>
      <Button
        label={item.label}
        tone="secondary"
        aria-current={item.id === activePage ? 'page' : undefined}
        class={`w-full ${item.id === activePage ? 'border-accent bg-surface text-text' : ''}`}
        onclick={() => onNavigate(item.id)}
      />
    </li>
  {/each}
</ul>
