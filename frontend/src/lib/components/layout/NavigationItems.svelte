<script module lang="ts">
  export type PageId = 'overview' | 'plugins' | 'settings' | 'diagnostics';

  export interface NavigationItem {
    id: PageId;
    label: string;
  }
</script>

<script lang="ts">
  import {Activity, Blocks, LayoutDashboard, Settings2} from 'lucide-svelte';
  import {Button} from '../ui/index.js';

  type Props = {
    items: NavigationItem[];
    activePage: PageId;
    onNavigate: (page: PageId) => void;
    class?: string;
  };
  const icons = {overview: LayoutDashboard, plugins: Blocks, settings: Settings2, diagnostics: Activity};
  let {items, activePage, onNavigate, class: className = ''}: Props = $props();
</script>

<ul class={`flex min-w-max items-center justify-start gap-2 px-3 py-2 ${className}`}>
  {#each items as item (item.id)}
    {@const Icon = icons[item.id]}
    <li class="shrink-0">
      <Button
        label={item.label}
        tone={item.id === activePage ? "primary" : "secondary"}
        aria-current={item.id === activePage ? 'page' : undefined}
        class="min-h-10 whitespace-nowrap"
        onclick={() => onNavigate(item.id)}
      >
        {#snippet children()}
          <Icon size={18} aria-hidden="true" />
          <span>{item.label}</span>
        {/snippet}
      </Button>
    </li>
  {/each}
</ul>
