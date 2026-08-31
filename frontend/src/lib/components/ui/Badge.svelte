<script lang="ts">
  type Tone = 'neutral' | 'success' | 'warning' | 'danger';
  type Props = {label: string; tone?: Tone};

  const toneClasses: Record<Tone, string> = {
    neutral: 'border-border bg-surface-raised text-text',
    success: 'border-accent bg-accent/15 text-accent',
    warning: 'border-warning bg-warning/15 text-warning',
    danger: 'border-danger bg-danger/15 text-danger',
  };

  let {label, tone = 'neutral'}: Props = $props();
</script>

<span class={`status-pill ${toneClasses[tone]}`} data-tone={tone}>{label}</span>
