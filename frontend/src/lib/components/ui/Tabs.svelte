<script module lang="ts">
  export interface TabItem<Value extends string> {
    value: Value;
    label: string;
    disabled?: boolean;
  }
</script>

<script lang="ts" generics="T extends string">
  import type {Snippet} from 'svelte';
  import {Tabs as BitsTabs} from 'bits-ui';

  type Props = {
    items: TabItem<T>[];
    value?: T;
    content?: Snippet<[T]>;
    children?: Snippet;
  };

  let {
    items,
    value = $bindable<T | undefined>(items.find((item) => !item.disabled)?.value),
    content,
    children,
  }: Props = $props();
  let selectedValue = $derived(value);
</script>

{#if selectedValue}
  <BitsTabs.Root bind:value activationMode="automatic" class="min-w-0">
    <BitsTabs.List class="flex min-w-0 gap-1 overflow-x-auto rounded-lg border border-border bg-surface p-1">
      {#each items as item (item.value)}
        <BitsTabs.Trigger
          value={item.value}
          disabled={item.disabled}
          class="focus-ring min-w-max rounded-md px-3 py-2 font-semibold text-text-muted transition-colors data-[state=active]:bg-surface-raised data-[state=active]:text-text disabled:cursor-not-allowed disabled:opacity-60"
        >
          {item.label}
        </BitsTabs.Trigger>
      {/each}
    </BitsTabs.List>
    <BitsTabs.Content value={selectedValue} class="min-w-0 pt-3">
      {#if content}
        {@render content(selectedValue)}
      {:else if children}
        {@render children()}
      {/if}
    </BitsTabs.Content>
  </BitsTabs.Root>
{/if}
