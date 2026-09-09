<script lang="ts">
  import {Button, TextField} from '../lib/components/ui/index.js';
  import {copyText} from '../lib/presentation/clipboard.js';
  import {
    AvatarSummary,
    DetailList,
    EmptyState,
    FormRow,
    FormSection,
    OSCSummary,
    PluginCard,
    ProblemBanner,
    StatusCard,
    UnsavedChangesBar,
  } from '../lib/patterns/index.js';

  if (!import.meta.env.DEV || !__COMPONENT_WORKBENCH__) {
    throw new Error('Component Workbench is development-only');
  }

  const longId = 'avtr_'.padEnd(180, 'a');
  let pluginEnabled = $state(false);
</script>

<main class="mx-auto grid min-w-0 max-w-6xl gap-6 p-4" aria-labelledby="workbench-title">
  <header class="grid min-w-0 gap-1">
    <h1 class="text-2xl font-bold text-text" id="workbench-title">Component Workbench</h1>
    <p class="text-text-muted">仅用于开发时检查共享组件状态。</p>
  </header>

  <section class="grid min-w-0 gap-3" aria-labelledby="fixture-normal">
    <h2 class="text-lg font-semibold text-text" id="fixture-normal">常规</h2>
    <StatusCard title="运行状态" label="等待连接" tone="neutral" detail="所有服务都在等待连接。" />
    <OSCSummary state="discovered" host="127.0.0.1" port={9000} />
  </section>

  <section class="grid min-w-0 gap-3" aria-labelledby="fixture-focus">
    <h2 class="text-lg font-semibold text-text" id="fixture-focus">焦点</h2>
    <Button label="焦点示例" class="ring-2 ring-accent ring-offset-2 ring-offset-canvas" />
    <TextField label="焦点输入" value="可编辑内容" />
  </section>

  <section class="grid min-w-0 gap-3" aria-labelledby="fixture-disabled">
    <h2 class="text-lg font-semibold text-text" id="fixture-disabled">已禁用</h2>
    <Button label="不可用操作" disabled />
  </section>

  <section class="grid min-w-0 gap-3" aria-labelledby="fixture-loading">
    <h2 class="text-lg font-semibold text-text" id="fixture-loading">加载中</h2>
    <StatusCard title="扫描插件" label="加载中" tone="neutral" loading loadingLabel="正在扫描插件" />
    <PluginCard id="example.loading" name="示例插件" enabled active state="running" capabilities={['eyes']} frameRate={60} restartCount={2} loading onCommand={({enabled}) => { pluginEnabled = enabled }} />
  </section>

  <section class="grid min-w-0 gap-3" aria-labelledby="fixture-empty">
    <h2 class="text-lg font-semibold text-text" id="fixture-empty">空状态</h2>
    <EmptyState title="没有可用插件" description="安装插件后会显示在这里。" />
  </section>

  <section class="grid min-w-0 gap-3" aria-labelledby="fixture-error">
    <h2 class="text-lg font-semibold text-text" id="fixture-error">错误</h2>
    <ProblemBanner title="无法保存设置" detail="请检查输入后重试。" tone="danger" diagnosticCode="validation" onCopyDiagnostic={copyText} />
  </section>

  <section class="grid min-w-0 gap-3" aria-labelledby="fixture-form-patterns">
    <h2 class="text-lg font-semibold text-text" id="fixture-form-patterns">表单和详细信息</h2>
    <FormSection title="工作台表单">
      <FormRow label="本地地址" description="用于本机 OSC 输出。">
        <TextField label="本地地址" value="127.0.0.1" />
      </FormRow>
    </FormSection>
    <DetailList items={[{label: '输出端口', value: '9000'}]} />
  </section>

  <section class="grid min-w-0 gap-3" aria-labelledby="fixture-long-text">
    <h2 class="text-lg font-semibold text-text" id="fixture-long-text">长文本</h2>
    <AvatarSummary name="" id={longId} />
  </section>

  <section class="grid min-w-0 gap-3" aria-labelledby="fixture-narrow">
    <h2 class="text-lg font-semibold text-text" id="fixture-narrow">320px 容器</h2>
    <div class="w-80 max-w-full min-w-0" data-testid="workbench-320px">
      <PluginCard id="example.narrow" name="窄容器中的示例插件" description="此卡片用于检查长名称和操作在 320px 容器中的表现。" enabled={pluginEnabled} state="backoff" capabilities={['eyes']} restartCount={2} error="等待重新连接" onCommand={({enabled}) => { pluginEnabled = enabled }} />
      <UnsavedChangesBar onSave={() => {}} onDiscard={() => {}} />
    </div>
  </section>
</main>
