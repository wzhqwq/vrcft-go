<script lang="ts">
  import {Button, TextField} from '../../components/ui/index.js'

  type Props = {
    values: readonly string[];
    id?: string;
    error?: string;
    onChange: (values: string[]) => void;
  }

  let {values, id = 'plugin-dev-roots', error, onChange}: Props = $props()

  function change(index: number, nextValue: string) {
    onChange(values.map((value, current) => current === index ? nextValue : value))
  }

  function remove(index: number) {
    onChange(values.filter((_, current) => current !== index))
  }
</script>

<fieldset class="grid min-w-0 gap-3" id={id} tabindex="-1" aria-describedby={error ? `${id}-error` : undefined}>
  <legend class="font-semibold text-text">插件开发目录</legend>
  <p class="text-sm text-text-muted">仅添加用于开发和测试本地插件的绝对目录。</p>
  {#each values as value, index (`${index}-${value}`)}
    <div class="flex min-w-0 flex-wrap items-end gap-2">
      <div class="min-w-0 flex-1">
        <TextField
          id={`${id}-${index}`}
          label={`开发目录 ${index}`}
          value={value}
          error={index === 0 ? error : undefined}
          oninput={(event) => change(index, event.currentTarget.value)}
        />
      </div>
      <Button label={`删除开发目录 ${index}`} tone="secondary" onclick={() => remove(index)} />
    </div>
  {/each}
  <Button label="添加开发目录" tone="secondary" class="justify-self-start" onclick={() => onChange([...values, ''])} />
  {#if error}<p class="text-sm text-danger" id={`${id}-error`} role="alert">{error}</p>{/if}
</fieldset>
