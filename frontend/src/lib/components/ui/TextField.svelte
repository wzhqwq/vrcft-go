<script lang="ts">
  import type {HTMLInputAttributes} from 'svelte/elements';

  type Props = Omit<HTMLInputAttributes, 'id' | 'value' | 'type'> & {
    label: string;
    description?: string;
    error?: string;
    value?: string;
    id?: string;
  };

  let {
    label,
    description,
    error,
    value = $bindable(''),
    id = globalThis.crypto?.randomUUID?.() ?? `field-${Math.random().toString(36).slice(2)}`,
    required = false,
    disabled = false,
    ...rest
  }: Props = $props();
  let descriptionId = $derived(`${id}-description`);
  let errorId = $derived(`${id}-error`);
</script>

<div class="grid min-w-0 gap-1.5">
  <label class="min-w-0 font-semibold text-text" for={id}>
    {label}{#if required}<span aria-hidden="true" class="ml-1 text-text-muted">（必填）</span>{/if}
  </label>
  {#if description}<p class="text-sm text-text-muted" id={descriptionId}>{description}</p>{/if}
  <input
    {...rest}
    {id}
    bind:value
    class="form-control focus-ring disabled:cursor-not-allowed disabled:opacity-60"
    aria-describedby={[description && descriptionId, error && errorId].filter(Boolean).join(' ') || undefined}
    aria-invalid={error ? 'true' : undefined}
    {required}
    {disabled}
  />
  {#if error}<p class="text-sm text-danger" id={errorId} role="alert">{error}</p>{/if}
</div>
