<script module lang="ts">
  export interface SelectOption<Value extends string> {
    value: Value;
    label: string;
    description?: string;
    disabled?: boolean;
  }
</script>

<script lang="ts" generics="T extends string">
  import {Select} from 'bits-ui';
  import {ChevronDown} from 'lucide-svelte';

  type Props = {
    label: string;
    options: SelectOption<T>[];
    description?: string;
    error?: string;
    placeholder?: string;
    value?: T;
    onValueChange?: (value: T) => void;
    id?: string;
    name?: string;
    required?: boolean;
    disabled?: boolean;
  };

  let {
    label,
    options,
    description,
    error,
    placeholder = '请选择',
    value = $bindable<T | undefined>(undefined),
    onValueChange,
    id = globalThis.crypto?.randomUUID?.() ?? `field-${Math.random().toString(36).slice(2)}`,
    name,
    required = false,
    disabled = false,
  }: Props = $props();
  let labelId = $derived(`${id}-label`);
  let descriptionId = $derived(`${id}-description`);
  let errorId = $derived(`${id}-error`);
</script>

<div class="grid min-w-0 gap-1.5">
  <p class="min-w-0 font-semibold text-text" id={labelId}>{label}{#if required}<span aria-hidden="true" class="ml-1 text-text-muted">（必填）</span>{/if}</p>
  {#if description}<p class="text-sm text-text-muted" id={descriptionId}>{description}</p>{/if}
  <Select.Root bind:value={() => value, (next) => { value = next; if (next !== undefined) onValueChange?.(next) }} items={options} type="single" {disabled} {name} {required}>
    <Select.Trigger
      {id}
      aria-describedby={[description && descriptionId, error && errorId].filter(Boolean).join(' ') || undefined}
      aria-invalid={error ? 'true' : undefined}
      aria-labelledby={labelId}
      aria-required={required || undefined}
      class="group form-control focus-ring flex items-center justify-between gap-3 text-left data-[placeholder]:text-text-muted disabled:cursor-not-allowed disabled:opacity-60"
    >
      <Select.Value {placeholder} />
      <ChevronDown aria-hidden="true" class="size-4 shrink-0 text-text-muted transition-transform group-data-[state=open]:rotate-180" />
    </Select.Trigger>
    <Select.Portal>
      <Select.Content class="z-50 max-h-72 w-[var(--bits-floating-anchor-width)] min-w-[var(--bits-floating-anchor-width)] overflow-auto rounded-lg border border-border bg-surface p-1 shadow-lg shadow-shadow/30">
        <Select.Viewport>
          {#each options as option (option.value)}
            <Select.Item
              value={option.value}
              label={option.label}
              disabled={option.disabled}
              class="flex min-w-0 cursor-default flex-col rounded-md px-3 py-2 text-text outline-none data-[highlighted]:bg-surface-raised data-[disabled]:cursor-not-allowed data-[disabled]:opacity-60"
            >
              <span>{option.label}</span>
              {#if option.description}<span class="text-sm text-text-muted">{option.description}</span>{/if}
            </Select.Item>
          {/each}
        </Select.Viewport>
      </Select.Content>
    </Select.Portal>
  </Select.Root>
  {#if error}<p class="text-sm text-danger" id={errorId} role="alert">{error}</p>{/if}
</div>
