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
  class={`grid h-dvh min-w-0 grid-rows-[auto_minmax(0,1fr)] ${className}`}
>
  <nav class="min-w-0 overflow-x-auto border-b border-border bg-surface" aria-label="主导航">
    <NavigationItems items={navigation} {activePage} {onNavigate} />
  </nav>
  <div class="min-h-0 min-w-0 overflow-y-auto p-3">{@render content()}</div>
</div>
