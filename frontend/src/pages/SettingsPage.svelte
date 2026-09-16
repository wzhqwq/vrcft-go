<script lang="ts">
  import {copy} from '../copy/zh-CN.js'
  import {copyText} from '../lib/presentation/clipboard.js'
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
  import {fieldTargets, type SettingsField} from '../lib/modules/settings/index.js'
  import type {SettingsModule} from '../lib/modules/settings/types.js'

  type Section = 'general' | 'processing' | 'osc'
  type Props = {settings: SettingsModule}

  const sections = [
    {value: 'general', label: copy.text.general},
    {value: 'processing', label: copy.text.processing},
    {value: 'osc', label: 'OSC'},
  ] satisfies Array<{value: Section; label: string}>
  const targetModes = [
    {value: 'auto', label: copy.text.automatic},
    {value: 'manual', label: copy.text.manual},
  ]

  let {settings}: Props = $props()
  let section = $state<Section>('general')
  let routedField = $state<SettingsField | null>(null)
  let confirmReload = $state(false)

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
    if (settings.state.conflict) confirmReload = true
    else void settings.reload()
  }

  function reloadAfterConfirmation() {
    confirmReload = false
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

<main class="page-grid min-w-0" aria-label={copy.navigation.settings}>
  <PageHeader title={copy.navigation.settings} description={copy.text.settingsDescription} />

  {#if settings.state.status === 'loading'}
    <StatusCard title={copy.navigation.settings} label={copy.state.loading} tone="neutral" loading loadingLabel={copy.text.readSettings} />
  {:else if settings.state.draft === null}
    {#if settings.state.problem}
      <ProblemBanner title={settings.state.problem.title} detail={settings.state.problem.detail} tone={settings.state.problem.tone} diagnosticCode={settings.state.problem.code} onCopyDiagnostic={copyText} />
    {/if}
    <section class="surface-card grid min-w-0 gap-1" aria-label={copy.text.settingsUnavailable}>
      <h2 class="text-lg font-semibold text-text">{copy.text.settingsUnavailable}</h2>
      <p class="text-text-muted">{copy.text.settingsUnavailableDetail}</p>
    </section>
  {:else}
    {@const draft = settings.state.draft}
    <Stack gap="lg">
      {#if settings.state.status === 'stale'}
        <p role="status" class="text-warning">{copy.format.staleSettings(settings.state.updatedAt)}</p>
      {/if}
      {#if settings.state.problem && !settings.state.conflict}
        <ProblemBanner
          title={settings.state.problem.title}
          detail={settings.state.problem.detail}
          tone={settings.state.problem.tone}
          diagnosticCode={settings.state.problem.code} onCopyDiagnostic={copyText}
        />
      {/if}

      {#if settings.state.conflict}
        <section class="surface-card grid min-w-0 gap-3" aria-label={copy.text.settingsConflict}>
          <div class="grid min-w-0 gap-1">
            <h2 class="text-lg font-semibold text-text">{copy.text.settingsChanged}</h2>
            <p class="text-text-muted">{copy.text.reloadWarning}</p>
          </div>
          <Button label={copy.text.reloadSettings} tone="secondary" onclick={discard} />
        </section>
      {/if}

      {#if settings.state.restartRequired}
        <p class="rounded-lg border border-success bg-success/10 px-4 py-3 text-success" role="status">{copy.settings.savedRestart}</p>
      {/if}

      <Tabs items={sections} bind:value={section}>
        {#if section === 'general'}
          <FormSection title={copy.text.generalSettings} description={copy.text.generalDescription}>
            <TextField
              id="avatar-osc-root"
              label={copy.text.avatarRoot}
              value={draft.avatar.oscRoot}
              error={error('avatar.oscRoot')}
              oninput={(event) => updateDraft((next) => { next.avatar.oscRoot = event.currentTarget.value })}
              onblur={() => validate('avatar.oscRoot')}
            />
            <TextField
              id="avatar-fallback-path"
              label={copy.text.avatarFallback}
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
          <div id="processing-summary" tabindex="-1" role="group" aria-label={copy.text.processingProblems} class="focus-ring min-w-0">
            {#if error('processing')}<p role="alert" class="text-danger">{error('processing')}</p>{/if}
          </div>
          <FormSection title={copy.text.processingProfiles} description={copy.text.processingProfilesDescription}>
            <NumberField
              id="active-stale-after"
              label={copy.text.activeStale}
              value={draft.processing.activeStaleAfterMs}
              error={error('processing.activeStaleAfterMs')}
              min={0}
              step={1}
              oninput={(event) => updateDraft((next) => { next.processing.activeStaleAfterMs = event.currentTarget.valueAsNumber })}
              onblur={() => validate('processing.activeStaleAfterMs')}
            />
            <div onfocusout={(event) => validateOnLeave('processing', event)}>
              <ChannelOverridesEditor
                id="channel-overrides"
                defaultId="default-channel"
                defaultChannel={draft.processing.defaultChannel}
                values={draft.processing.overrides}
                error={error('processing.overrides')}
                onDefaultChange={(value) => updateDraft((next) => { next.processing.defaultChannel = value })}
                onChange={(values) => updateDraft((next) => { next.processing.overrides = values })}
              />
            </div>
            {#if error('processing.defaultChannel')}<p class="text-sm text-danger" role="alert">{error('processing.defaultChannel')}</p>{/if}
          </FormSection>
          <FormSection title={copy.text.mutualExclusion} description={copy.text.mutualDescription}>
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
          <FormSection title={copy.text.oscTarget} description={copy.text.oscTargetDescription}>
            <SelectField
              id="osc-target-mode"
              label={copy.text.targetMode}
              value={draft.osc.targetMode}
              options={targetModes}
              error={error('osc.targetMode')}
              onValueChange={(value) => updateDraft((next) => { next.osc.targetMode = value })}
            />
            {#if draft.osc.targetMode === 'auto' || error('osc.preferredService')}
              <div id="osc-preferred-service" role="group" aria-label={copy.text.preferredService} tabindex="-1">
                <TextField
                  id="osc-preferred-service-input"
                  label={copy.text.preferredService}
                  value={draft.osc.preferredService}
                  error={error('osc.preferredService')}
                  description={copy.text.preferredDescription}
                  oninput={(event) => updateDraft((next) => { next.osc.preferredService = event.currentTarget.value })}
                  onblur={() => validate('osc.preferredService')}
                />
              </div>
            {/if}
            {#if draft.osc.targetMode === 'manual' || error('osc.manualHost')}
              <TextField
                id="osc-manual-host"
                label={copy.text.manualHost}
                value={draft.osc.manualHost}
                error={error('osc.manualHost')}
                description={copy.text.manualHostDescription}
                oninput={(event) => updateDraft((next) => { next.osc.manualHost = event.currentTarget.value })}
                onblur={() => validate('osc.manualHost')}
              />
            {/if}
            {#if draft.osc.targetMode === 'manual' || error('osc.manualPort')}
              <NumberField
                id="osc-manual-port"
                label={copy.text.manualPort}
                value={draft.osc.manualPort}
                error={error('osc.manualPort')}
                min={1}
                max={65535}
                step={1}
                description={copy.text.manualPortDescription}
                oninput={(event) => updateDraft((next) => { next.osc.manualPort = event.currentTarget.valueAsNumber })}
                onblur={() => validate('osc.manualPort')}
              />
            {/if}
          </FormSection>
        {/if}
      </Tabs>

      {#if settings.state.dirty}
        <UnsavedChangesBar
          message={copy.text.dirty}
          saving={settings.state.saving}
          canSave={settings.state.canSave}
          onSave={save}
          onDiscard={discard}
        />
      {/if}
    </Stack>
  {/if}
</main>
<Dialog triggerLabel={copy.text.reloadSettings} title={copy.text.reloadSettings} description={copy.text.reloadConfirmDescription} closeLabel={copy.actions.cancel} showTrigger={false} bind:open={confirmReload}>
  <Button label={copy.text.confirmReload} tone="danger" onclick={reloadAfterConfirmation} />
</Dialog>
