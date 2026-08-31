<script lang="ts">
  import {IconButton, Tooltip} from '../index.js';
</script>

<Tooltip content="重新加载状态" delayDuration={0}>
  {#snippet trigger(props)}
    <IconButton label="刷新" {...props} />
  {/snippet}
</Tooltip>
