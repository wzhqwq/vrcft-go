import {fireEvent, render, screen} from '@testing-library/svelte';
import {createRawSnippet} from 'svelte';
import {describe, expect, it} from 'vitest';

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
} from './index.js';

describe('shared UI patterns', () => {
  it('keeps a long Avatar ID shrinkable while retaining an available copy action', () => {
    const longId = 'avtr_'.padEnd(180, 'a');

    render(AvatarSummary, {props: {name: '', id: longId}});

    expect(screen.getByText(longId)).toHaveClass('min-w-0');
    expect(screen.getByRole('button', {name: '复制 Avatar ID'})).toBeEnabled();
  });

  it('announces loading and error status with visible text', () => {
    render(StatusCard, {props: {title: '运行状态', loading: true, loadingLabel: '正在检查'}});
    render(StatusCard, {props: {title: '运行状态', tone: 'danger', detail: '无法连接到服务'}});

    expect(screen.getByRole('status')).toHaveTextContent('正在检查');
    expect(screen.getByText('无法连接到服务')).toBeVisible();
  });

  it('renders an explicit OSC state and only its output target host and port', () => {
    render(OSCSummary, {props: {state: 'discovered', host: '127.0.0.1', port: 9000}});

    expect(screen.getByText('已发现 OSC 输出')).toBeVisible();
    expect(screen.getByText('127.0.0.1:9000')).toBeVisible();
  });

  it('sends plugin enablement commands for the plugin that owns the action', async () => {
    const commands: Array<{pluginId: string; enabled: boolean}> = [];
    render(PluginCard, {
      props: {
        id: 'tracking.vendor',
        name: 'Vendor Tracking',
        enabled: true,
        onCommand: (command) => commands.push(command),
      },
    });

    await fireEvent.click(screen.getByRole('button', {name: '停用插件'}));

    expect(commands).toEqual([{pluginId: 'tracking.vendor', enabled: false}]);
  });

  it('keeps unsaved actions sticky inside the content area', () => {
    render(UnsavedChangesBar, {props: {onSave: () => {}, onDiscard: () => {}}});

    expect(screen.getByRole('region', {name: '未保存的更改'})).toHaveClass('sticky', 'bottom-0');
    expect(screen.getByRole('button', {name: '保存更改'})).toBeEnabled();
    expect(screen.getByRole('button', {name: '放弃更改'})).toBeEnabled();
  });

  it('copies only a safe diagnostic code instead of error detail', async () => {
    const copied: string[] = [];
    render(ProblemBanner, {
      props: {
        title: '操作失败',
        detail: 'token=super-secret',
        tone: 'danger',
        diagnosticCode: 'internal',
        onCopyDiagnostic: (diagnostic) => copied.push(diagnostic),
      },
    });

    await fireEvent.click(screen.getByRole('button', {name: '复制诊断信息'}));

    expect(copied).toEqual(['问题代码：internal']);
    expect(copied[0]).not.toContain('super-secret');
  });

  it('renders empty content, labelled form content, and details without product data dependencies', () => {
    const content = createRawSnippet(() => ({render: () => '<input aria-label="显示名称" />'}));
    render(EmptyState, {props: {title: '没有可用插件', description: '安装插件后会显示在这里。'}});
    render(FormSection, {props: {title: '基本设置', children: content}});
    render(FormRow, {props: {label: '显示名称', children: content}});
    render(DetailList, {props: {items: [{label: '输出端口', value: '9000'}]}});

    expect(screen.getByText('没有可用插件')).toBeVisible();
    expect(screen.getByRole('region', {name: '基本设置'})).toBeVisible();
    expect(screen.getAllByRole('textbox', {name: '显示名称'})).toHaveLength(2);
    expect(screen.getByText('输出端口')).toBeVisible();
    expect(screen.getByText('9000')).toBeVisible();
  });
});
