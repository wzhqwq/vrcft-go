<script lang="ts">
  type Props = {orientation?: 'horizontal' | 'vertical'; decorative?: boolean};
  let {orientation = 'horizontal', decorative = false}: Props = $props();
</script>

<div
  class={orientation === 'horizontal' ? 'h-px w-full shrink-0 bg-border' : 'h-full min-h-4 w-px shrink-0 bg-border'}
  aria-hidden={decorative ? 'true' : undefined}
  aria-orientation={decorative ? undefined : orientation}
  role={decorative ? undefined : 'separator'}
></div>
