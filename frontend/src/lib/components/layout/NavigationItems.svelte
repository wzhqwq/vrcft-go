<script module lang="ts">
  export type PageId = 'overview' | 'plugins' | 'settings' | 'diagnostics';

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
    horizontal: 'flex min-w-max w-full items-center gap-2 px-4 py-2',
    vertical: 'flex min-w-0 flex-col gap-2 p-4',
  };

  let {items, activePage, onNavigate, orientation, class: className = ''}: Props = $props();
</script>

<ul class={`${orientationClasses[orientation]} ${className}`}>
  {#each items as item (item.id)}
    <li class={orientation === 'vertical' ? 'min-w-0' : 'min-w-32 flex-1'}>
      <Button
        label={item.label}
        tone={item.id === activePage ? "primary" : "secondary"}
        aria-current={item.id === activePage ? 'page' : undefined}
        class="w-full"
        onclick={() => onNavigate(item.id)}
      />
    </li>
  {/each}
</ul>
