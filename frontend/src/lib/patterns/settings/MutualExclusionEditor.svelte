<script lang="ts">
  import {copy} from '../../../copy/zh-CN.js'
  import {Button, TextField} from '../../components/ui/index.js'
  import {createRowKeys} from './row-keys.js'

  type Props = {
    values: readonly (readonly string[])[];
    id?: string;
    error?: string;
    onChange: (values: string[][]) => void;
  }

  let {values, id = 'mutual-exclusion', error, onChange}: Props = $props()
  const rowKeys = createRowKeys()
  let rawText = $state<Record<number, string>>({})

  function text(index: number, group: readonly string[]): string {
    const raw = rawText[rowKeys.at(index)]
    return raw !== undefined && JSON.stringify(members(raw)) === JSON.stringify(group) ? raw : group.join(', ')
  }

  function change(index: number, raw: string) {
    rawText[rowKeys.at(index)] = raw
    onChange(values.map((current, currentIndex) => currentIndex === index ? members(raw) : [...current]))
  }

  function remove(index: number) {
    delete rawText[rowKeys.at(index)]
    rowKeys.remove(index)
    onChange(values.filter((_, current) => current !== index).map((group) => [...group]))
  }

  function members(value: string): string[] {
    return value.split(',').map((member) => member.trim()).filter((member) => member !== '')
  }
</script>

<fieldset class="grid min-w-0 gap-3" id={id} tabindex="-1" aria-describedby={error ? `${id}-error` : undefined}>
  <legend class="font-semibold text-text">{copy.text.mutualExclusion}</legend>
  <p class="text-sm text-text-muted">{copy.text.mutualEditorDescription}</p>
  {#each values as group, index (rowKeys.at(index))}
    <div class="flex min-w-0 flex-wrap items-end gap-2" role="group" aria-label={copy.format.group(index)}>
      <div class="min-w-0 flex-1">
        <TextField id={`${id}-${rowKeys.at(index)}`} label={copy.format.groupMembers(index)} value={text(index, group)} oninput={(event) => change(index, event.currentTarget.value)} />
      </div>
      <Button label={copy.format.removeGroup(index)} tone="secondary" onclick={() => remove(index)} />
    </div>
  {/each}
  <Button label={copy.text.addGroup} tone="secondary" class="justify-self-start" onclick={() => onChange([...values.map((group) => [...group]), []])} />
  {#if error}<p class="text-sm text-danger" id={`${id}-error`} role="alert">{error}</p>{/if}
</fieldset>
