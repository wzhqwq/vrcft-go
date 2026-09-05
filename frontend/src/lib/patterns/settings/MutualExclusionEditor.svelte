<script lang="ts">
  import {Button, TextField} from '../../components/ui/index.js'

  type Props = {
    values: readonly (readonly string[])[];
    id?: string;
    error?: string;
    onChange: (values: string[][]) => void;
  }

  let {values, id = 'mutual-exclusion', error, onChange}: Props = $props()

  function members(value: string): string[] {
    return value.split(',').map((member) => member.trim()).filter((member) => member !== '')
  }
</script>

<fieldset class="grid min-w-0 gap-3" id={id} tabindex="-1" aria-describedby={error ? `${id}-error` : undefined}>
  <legend class="font-semibold text-text">互斥组</legend>
  <p class="text-sm text-text-muted">同一组中的通道不会同时输出。</p>
  {#each values as group, index (`${index}-${group.join('|')}`)}
    <div class="flex min-w-0 flex-wrap items-end gap-2" role="group" aria-label={`互斥组 ${index}`}>
      <div class="min-w-0 flex-1">
        <TextField id={`${id}-${index}`} label={`互斥组 ${index} 成员`} value={group.join(', ')} oninput={(event) => onChange(values.map((current, currentIndex) => currentIndex === index ? members(event.currentTarget.value) : [...current]))} />
      </div>
      <Button label={`删除互斥组 ${index}`} tone="secondary" onclick={() => onChange(values.filter((_, current) => current !== index).map((group) => [...group]))} />
    </div>
  {/each}
  <Button label="添加互斥组" tone="secondary" class="justify-self-start" onclick={() => onChange([...values.map((group) => [...group]), []])} />
  {#if error}<p class="text-sm text-danger" id={`${id}-error`} role="alert">{error}</p>{/if}
</fieldset>
