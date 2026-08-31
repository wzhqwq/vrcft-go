<script lang="ts">
  import {Switch} from 'bits-ui';

  type Props = {
    label: string;
    description?: string;
    error?: string;
    checked?: boolean;
    id?: string;
    name?: string;
    required?: boolean;
    disabled?: boolean;
  };

  let {
    label,
    description,
    error,
    checked = $bindable(false),
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
  <div class="flex min-w-0 items-center justify-between gap-3">
    <div class="min-w-0">
      <p class="font-semibold text-text" id={labelId}>{label}{#if required}<span aria-hidden="true" class="ml-1 text-text-muted">（必填）</span>{/if}</p>
      {#if description}<p class="text-sm text-text-muted" id={descriptionId}>{description}</p>{/if}
    </div>
    <Switch.Root
      bind:checked
      {disabled}
      {name}
      {required}
      aria-describedby={[description && descriptionId, error && errorId].filter(Boolean).join(' ') || undefined}
      aria-invalid={error ? 'true' : undefined}
      aria-labelledby={labelId}
      aria-required={required || undefined}
      class="focus-ring inline-flex h-6 w-11 shrink-0 items-center rounded-full border border-border bg-surface-raised p-0.5 transition-colors data-[state=checked]:border-accent data-[state=checked]:bg-accent disabled:cursor-not-allowed disabled:opacity-60"
    >
      <Switch.Thumb class="size-5 rounded-full bg-text transition-transform data-[state=checked]:translate-x-5" />
    </Switch.Root>
  </div>
  {#if error}<p class="text-sm text-danger" id={errorId} role="alert">{error}</p>{/if}
</div>
