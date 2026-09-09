<script lang="ts">
  import SelectField from '../SelectField.svelte';
  let value = $state('auto');
  let calls = $state(0);
</script>
<SelectField label="模式" options={[{value: 'auto', label: '自动'}, {value: 'manual', label: '手动'}]} bind:value onValueChange={() => calls += 1} />
<output aria-label="选择结果">{value}</output>
<output aria-label="回调次数">{calls}</output>
