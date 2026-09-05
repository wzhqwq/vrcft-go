<script lang="ts">
  import {tick} from 'svelte'

  import {Button, Dialog, NumberField, SelectField, Tabs, TextField} from '../lib/components/ui/index.js'
  import {PageHeader, Stack} from '../lib/components/layout/index.js'
  import {
    FormSection,
    ProblemBanner,
    StatusCard,
    UnsavedChangesBar,
  } from '../lib/patterns/index.js'
  import ChannelOverridesEditor from '../lib/patterns/settings/ChannelOverridesEditor.svelte'
  import MutualExclusionEditor from '../lib/patterns/settings/MutualExclusionEditor.svelte'
  import PathListField from '../lib/patterns/settings/PathListField.svelte'
  import ProcessingChannelFields from '../lib/patterns/settings/ProcessingChannelFields.svelte'
  import {fieldTargets, type SettingsField} from '../lib/modules/settings/index.js'
  import type {SettingsModule} from '../lib/modules/settings/types.js'

  type Section = 'general' | 'processing' | 'osc'
  type Props = {settings: SettingsModule}

  const sections = [
    {value: 'general', label: '常规'},
    {value: 'processing', label: '处理'},
    {value: 'osc', label: 'OSC'},
  ] satisfies Array<{value: Section; label: string}>
  const targetModes = [
    {value: 'auto', label: '自动'},
    {value: 'manual', label: '手动'},
  ]

  let {settings}: Props = $props()
  let section = $state<Section>('general')
  let routedField = $state<SettingsField | null>(null)

  export function canLeave(): boolean {
    return !settings.state.dirty
  }

  function error(field: SettingsField): string | undefined {
    return settings.state.fieldProblems.get(field)?.detail
  }

  function updateDraft(update: Parameters<SettingsModule['updateDraft']>[0]) {
    settings.updateDraft(update)
  }

  function validate(field: SettingsField) {
    void settings.validate(field)
  }

  function validateOnLeave(field: SettingsField, event: FocusEvent) {
    const group = event.currentTarget
    if (!(group instanceof HTMLElement) || !group.contains(event.relatedTarget as Node | null)) validate(field)
  }

  function save() {
    void settings.save()
  }

  function discard() {
    void settings.reload()
  }

  function reloadAfterConfirmation() {
    void settings.reload()
  }

  $effect(() => {
    const field = [...settings.state.fieldProblems.keys()][0]
    if (field === undefined) {
      routedField = null
      return
    }
    if (field === routedField) return
    routedField = field
    const target = fieldTargets[field]
    section = target.section
    void tick().then(() => document.getElementById(target.control)?.focus())
  })
</script>

<main class="page-grid min-w-0" aria-label="设置">
  <PageHeader title="设置" description="这些设置将在下次重启后应用。" />

  {#if settings.state.status === 'loading'}
    <StatusCard title="设置" loading loadingLabel="正在读取设置" />
  {:else if settings.state.draft === null}
    <section class="surface-card grid min-w-0 gap-1" aria-label="设置不可用">
      <h2 class="text-lg font-semibold text-text">设置不可用</h2>
      <p class="text-text-muted">设置数据暂时不可用，请稍后重试。</p>
    </section>
  {:else}
    {@const draft = settings.state.draft}
    <Stack gap="lg">
      {#if settings.state.problem && !settings.state.conflict}
        <ProblemBanner
          title={settings.state.problem.title}
          detail={settings.state.problem.detail}
          tone={settings.state.problem.tone}
          diagnosticCode={settings.state.problem.code}
        />
      {/if}

      {#if settings.state.conflict}
        <section class="surface-card grid min-w-0 gap-3" aria-label="设置冲突">
          <div class="grid min-w-0 gap-1">
            <h2 class="text-lg font-semibold text-text">设置已在其他位置更新</h2>
            <p class="text-text-muted">重新加载会放弃当前未保存的更改。</p>
          </div>
          <Dialog triggerLabel="重新加载设置" title="重新加载设置" description="确认后将使用最新保存的设置。" closeLabel="取消">
            <Button label="确认重新加载" tone="danger" onclick={reloadAfterConfirmation} />
          </Dialog>
        </section>
      {/if}

      {#if settings.state.restartRequired}
        <p class="rounded-lg border border-success bg-success/10 px-4 py-3 text-success" role="status">已保存，将在重启后生效</p>
      {/if}

      <Tabs items={sections} bind:value={section}>
        {#if section === 'general'}
          <FormSection title="常规设置" description="Avatar 配置与本地插件开发目录。">
            <TextField
              id="avatar-osc-root"
              label="Avatar OSC 根目录"
              value={draft.avatar.oscRoot}
              error={error('avatar.oscRoot')}
              oninput={(event) => updateDraft((next) => { next.avatar.oscRoot = event.currentTarget.value })}
              onblur={() => validate('avatar.oscRoot')}
            />
            <TextField
              id="avatar-fallback-path"
              label="Fallback Avatar 配置"
              value={draft.avatar.fallbackPath}
              error={error('avatar.fallbackPath')}
              oninput={(event) => updateDraft((next) => { next.avatar.fallbackPath = event.currentTarget.value })}
              onblur={() => validate('avatar.fallbackPath')}
            />
            <div onfocusout={(event) => validateOnLeave('plugins.devRoots', event)}>
              <PathListField
                id="plugin-dev-roots"
                values={draft.plugins.devRoots}
                error={error('plugins.devRoots')}
                onChange={(values) => updateDraft((next) => { next.plugins.devRoots = values })}
              />
            </div>
          </FormSection>
        {:else if section === 'processing'}
          <FormSection title="默认通道处理" description="校准、调节、滤波和丢失数据策略。">
            <NumberField
              id="active-stale-after"
              label="活跃通道过期时长（毫秒）"
              value={draft.processing.activeStaleAfterMs}
              error={error('processing.activeStaleAfterMs')}
              min={0}
              step={1}
              oninput={(event) => updateDraft((next) => { next.processing.activeStaleAfterMs = event.currentTarget.valueAsNumber })}
              onblur={() => validate('processing.activeStaleAfterMs')}
            />
            <div onfocusout={(event) => validateOnLeave('processing.defaultChannel', event)}>
              <ProcessingChannelFields
                id="default-channel"
                value={draft.processing.defaultChannel}
                onChange={(value) => updateDraft((next) => { next.processing.defaultChannel = value })}
              />
            </div>
            {#if error('processing.defaultChannel')}<p class="text-sm text-danger" role="alert">{error('processing.defaultChannel')}</p>{/if}
          </FormSection>
          <FormSection title="通道覆盖" description="为指定通道替换默认处理参数。">
            <div onfocusout={(event) => validateOnLeave('processing.overrides', event)}>
              <ChannelOverridesEditor
                id="channel-overrides"
                values={draft.processing.overrides}
                error={error('processing.overrides')}
                onChange={(values) => updateDraft((next) => { next.processing.overrides = values })}
              />
            </div>
          </FormSection>
          <FormSection title="互斥组" description="同组通道不会同时输出。">
            <div onfocusout={(event) => validateOnLeave('processing.mutualExclusion', event)}>
              <MutualExclusionEditor
                id="mutual-exclusion"
                values={draft.processing.mutualExclusion}
                error={error('processing.mutualExclusion')}
                onChange={(values) => updateDraft((next) => { next.processing.mutualExclusion = values })}
              />
            </div>
          </FormSection>
        {:else}
          <FormSection title="OSC 目标" description="选择自动发现或手动目标。">
            <SelectField
              id="osc-target-mode"
              label="目标模式"
              value={draft.osc.targetMode}
              options={targetModes}
              error={error('osc.targetMode')}
              onValueChange={(value) => updateDraft((next) => { next.osc.targetMode = value })}
            />
            <TextField
              id="osc-preferred-service"
              label="首选发现服务"
              value={draft.osc.preferredService}
              error={error('osc.preferredService')}
              disabled={draft.osc.targetMode !== 'auto'}
              description={draft.osc.targetMode === 'auto' ? '自动模式下优先使用此发现服务。' : '仅自动模式可编辑首选发现服务。'}
              oninput={(event) => updateDraft((next) => { next.osc.preferredService = event.currentTarget.value })}
              onblur={() => validate('osc.preferredService')}
            />
            <TextField
              id="osc-manual-host"
              label="手动主机"
              value={draft.osc.manualHost}
              error={error('osc.manualHost')}
              disabled={draft.osc.targetMode !== 'manual'}
              description={draft.osc.targetMode === 'manual' ? '手动模式下使用 IP 地址作为 OSC 目标。' : '仅手动模式可编辑主机和端口。'}
              oninput={(event) => updateDraft((next) => { next.osc.manualHost = event.currentTarget.value })}
              onblur={() => validate('osc.manualHost')}
            />
            <NumberField
              id="osc-manual-port"
              label="手动端口"
              value={draft.osc.manualPort}
              error={error('osc.manualPort')}
              min={1}
              max={65535}
              step={1}
              disabled={draft.osc.targetMode !== 'manual'}
              description={draft.osc.targetMode === 'manual' ? '端口必须在 1 到 65535 之间。' : '仅手动模式可编辑主机和端口。'}
              oninput={(event) => updateDraft((next) => { next.osc.manualPort = event.currentTarget.valueAsNumber })}
              onblur={() => validate('osc.manualPort')}
            />
          </FormSection>
        {/if}
      </Tabs>

      {#if settings.state.dirty}
        <UnsavedChangesBar
          message="有未保存的更改"
          saving={settings.state.saving}
          onSave={save}
          onDiscard={discard}
        />
      {/if}
    </Stack>
  {/if}
</main>
