<script lang="ts">
  import {copy} from '../../../copy/zh-CN.js'
  import {Button, TextField} from '../../components/ui/index.js'
  import {createRowKeys} from './row-keys.js'

  type Props = {
    values: readonly string[];
    id?: string;
    error?: string;
    onChange: (values: string[]) => void;
  }

  let {values, id = 'plugin-dev-roots', error, onChange}: Props = $props()
  const rowKeys = createRowKeys()

  function change(index: number, nextValue: string) {
    onChange(values.map((value, current) => current === index ? nextValue : value))
  }

  function remove(index: number) {
    rowKeys.remove(index)
    onChange(values.filter((_, current) => current !== index))
  }
</script>

<fieldset class="grid min-w-0 gap-3" id={id} tabindex="-1" aria-describedby={error ? `${id}-error` : undefined}>
  <legend class="font-semibold text-text">{copy.text.devRoots}</legend>
  <p class="text-sm text-text-muted">{copy.text.devRootsDescription}</p>
  {#each values as value, index (rowKeys.at(index))}
    <div class="flex min-w-0 flex-wrap items-end gap-2">
      <div class="min-w-0 flex-1">
        <TextField
          id={`${id}-${rowKeys.at(index)}`}
          label={copy.format.devRoot(index)}
          value={value}
          error={index === 0 ? error : undefined}
          oninput={(event) => change(index, event.currentTarget.value)}
        />
      </div>
      <Button label={copy.format.removeDevRoot(index)} tone="secondary" onclick={() => remove(index)} />
    </div>
  {/each}
  <Button label={copy.text.addDevRoot} tone="secondary" class="justify-self-start" onclick={() => onChange([...values, ''])} />
  {#if error}<p class="text-sm text-danger" id={`${id}-error`} role="alert">{error}</p>{/if}
</fieldset>
