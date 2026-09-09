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
  it('labels the public plugin handshaking state for users', () => {
    render(PluginCard, {props: {id: 'handshake', name: 'Tracker', enabled: true, state: 'handshaking'}});
    expect(screen.getByRole('article', {name: 'Tracker'})).toHaveTextContent('正在握手');
  });
  it('keeps a long Avatar ID shrinkable while retaining an available copy action', () => {
    const longId = 'avtr_'.padEnd(180, 'a');

    render(AvatarSummary, {props: {name: '', id: longId}});

    expect(screen.getAllByText(longId)).toHaveLength(2);
    for (const value of screen.getAllByText(longId)) expect(value).toHaveClass('min-w-0');
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

    await fireEvent.click(screen.getByRole('switch', {name: '启用 Vendor Tracking'}));

    expect(commands).toEqual([{pluginId: 'tracking.vendor', enabled: false}]);
  });

  it('never exposes an enabled diagnostic action when no copy command is provided', () => {
    render(ProblemBanner, {props: {title: '问题', detail: '安全摘要', tone: 'danger', diagnosticCode: 'internal'}});
    expect(screen.queryByRole('button', {name: '复制诊断信息'})).not.toBeInTheDocument();
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

  it('replaces invalid and oversized diagnostic codes before copying', async () => {
    const copied: string[] = [];
    const oversizedCode = 'a'.repeat(65);
    render(ProblemBanner, {
      props: {
        title: '操作失败',
        detail: '安全摘要',
        tone: 'danger',
        diagnosticCode: 'invalid code',
        onCopyDiagnostic: (diagnostic) => copied.push(diagnostic),
      },
    });
    render(ProblemBanner, {
      props: {
        title: '操作失败',
        detail: '安全摘要',
        tone: 'danger',
        diagnosticCode: oversizedCode,
        onCopyDiagnostic: (diagnostic) => copied.push(diagnostic),
      },
    });

    for (const button of screen.getAllByRole('button', {name: '复制诊断信息'})) {
      await fireEvent.click(button);
    }

    expect(copied).toEqual(['问题代码：unknown', '问题代码：unknown']);
  });

  it('generates distinct ARIA label targets for repeated pattern instances', () => {
    const content = createRawSnippet(() => ({render: () => '<span>表单内容</span>'}));
    const cases = [
      () => render(AvatarSummary, {props: {name: '一号', id: 'avtr_one'}}),
      () => render(OSCSummary, {props: {state: 'manual', host: '127.0.0.1', port: 9000}}),
      () => render(EmptyState, {props: {title: '空状态'}}),
      () => render(ProblemBanner, {props: {title: '问题', detail: '安全摘要', tone: 'warning'}}),
      () => render(FormSection, {props: {title: '表单', children: content}}),
    ];

    for (const createInstance of cases) {
      const first = createInstance();
      const second = createInstance();
      const targets = [
        first.container.querySelector('section')?.getAttribute('aria-labelledby'),
        second.container.querySelector('section')?.getAttribute('aria-labelledby'),
      ];
      expect(new Set(targets).size).toBe(2);
    }
  });

  it('uses caller-supplied IDs as stable ARIA label targets', () => {
    const content = createRawSnippet(() => ({render: () => '<span>表单内容</span>'}));
    const avatar = render(AvatarSummary, {props: {name: '一号', id: 'avtr_one', summaryId: 'avatar-summary-one'}});
    const osc = render(OSCSummary, {props: {id: 'osc-one', state: 'manual', host: '127.0.0.1', port: 9000}});
    const empty = render(EmptyState, {props: {id: 'empty-one', title: '空状态'}});
    const problem = render(ProblemBanner, {props: {id: 'problem-one', title: '问题', detail: '安全摘要', tone: 'warning'}});
    const form = render(FormSection, {props: {id: 'form-one', title: '表单', children: content}});

    const expectStableId = (container: HTMLElement, expected: string) => {
      expect(container.querySelector('section')).toHaveAttribute('aria-labelledby', expected);
      expect(container.querySelector(`#${expected}`)).toBeInTheDocument();
    };

    expectStableId(avatar.container, 'avatar-summary-one-title');
    expectStableId(osc.container, 'osc-one-title');
    expectStableId(empty.container, 'empty-one-title');
    expectStableId(problem.container, 'problem-one-title');
    expectStableId(form.container, 'form-one-title');
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
