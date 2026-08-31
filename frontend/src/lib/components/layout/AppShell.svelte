<script lang="ts">
  import type {Snippet} from 'svelte';

  import NavigationItems from './NavigationItems.svelte';
  import type {NavigationItem, PageId} from './NavigationItems.svelte';

  type Props = {
    navigation: NavigationItem[];
    activePage: PageId;
    onNavigate: (page: PageId) => void;
    content: Snippet;
    class?: string;
  };

  let {navigation, activePage, onNavigate, content, class: className = ''}: Props = $props();
</script>

<div
  data-testid="app-shell"
  class={`grid h-dvh min-w-0 grid-rows-[auto_auto_minmax(0,1fr)] nav:grid-cols-[14rem_minmax(0,1fr)] nav:grid-rows-[minmax(0,1fr)] ${className}`}
>
  <aside class="hidden min-w-0 flex-col border-r border-border bg-surface nav:flex">
    <NavigationItems items={navigation} {activePage} {onNavigate} orientation="vertical" />
  </aside>
  <header class="flex min-w-0 items-center border-b border-border bg-surface px-4 py-3 text-lg font-bold text-text nav:hidden">
    VRCFaceTracking
  </header>
  <nav class="min-w-0 overflow-x-auto border-b border-border bg-surface nav:hidden" aria-label="主导航">
    <NavigationItems items={navigation} {activePage} {onNavigate} orientation="horizontal" />
  </nav>
  <main class="min-h-0 min-w-0 overflow-y-auto p-4">{@render content()}</main>
</div>
