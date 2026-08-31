<script lang="ts">
  import {Dialog} from '../index.js';

  let open = $state(false);
</script>

<Dialog bind:open triggerLabel="打开" title="连接设置">
  {#snippet children()}
    <button type="button" onclick={() => open = false}>外部关闭</button>
  {/snippet}
</Dialog>
