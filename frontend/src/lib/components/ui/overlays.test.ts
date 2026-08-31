import {fireEvent, render, screen} from '@testing-library/svelte';
import {describe, expect, it} from 'vitest';

import {Collapsible, Dialog, Tabs, Tooltip} from './index.js';
import DialogHost from './test/DialogHost.svelte';
import TooltipIconHost from './test/TooltipIconHost.svelte';

describe('interactive controls', () => {
  it('moves between tabs with arrow keys', async () => {
    render(Tabs, {
      props: {
        value: 'runtime',
        items: [{value: 'runtime', label: '运行时'}, {value: 'plugins', label: '插件'}],
      },
    });

    const runtime = screen.getByRole('tab', {name: '运行时'});
    await fireEvent.click(runtime);
    await fireEvent.keyDown(runtime, {key: 'ArrowRight'});
    expect(screen.getByRole('tab', {name: '插件'})).toHaveAttribute('aria-selected', 'true');
  });

  it('activates the first enabled unbound tab and its panel', () => {
    render(Tabs, {
      props: {
        items: [
          {value: 'disabled', label: '不可用', disabled: true},
          {value: 'runtime', label: '运行时'},
          {value: 'plugins', label: '插件'},
        ],
      },
    });

    expect(screen.getByRole('tab', {name: '运行时'})).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('tabpanel')).toBeInTheDocument();
  });

  it('returns focus to the dialog trigger after Escape closes a controlled dialog', async () => {
    render(Dialog, {props: {triggerLabel: '打开', title: '连接设置'}});

    const trigger = screen.getByRole('button', {name: '打开'});
    trigger.focus();
    await fireEvent.click(trigger);
    expect(screen.getByRole('dialog', {name: '连接设置'})).toBeVisible();
    await fireEvent.keyDown(document, {key: 'Escape'});
    expect(trigger).toHaveFocus();
  });

  it('restores invoking focus after a host closes a bindable dialog externally', async () => {
    render(DialogHost);

    const trigger = screen.getByRole('button', {name: '打开'});
    trigger.focus();
    await fireEvent.click(trigger);
    await fireEvent.click(screen.getByRole('button', {name: '外部关闭'}));
    expect(screen.queryByRole('dialog', {name: '连接设置'})).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it('exposes tooltip text and collapsible state through accessible controls', async () => {
    render(Tooltip, {props: {triggerLabel: '帮助', content: '打开帮助文档', delayDuration: 0}});
    render(Collapsible, {props: {triggerLabel: '高级选项'}});

    await fireEvent.pointerEnter(screen.getByRole('button', {name: '帮助'}));
    expect(await screen.findByRole('tooltip')).toHaveTextContent('打开帮助文档');

    const trigger = screen.getByRole('button', {name: '高级选项'});
    await fireEvent.click(trigger);
    expect(trigger).toHaveAttribute('aria-expanded', 'true');
  });

  it('uses a tooltip trigger snippet with one labelled icon button', async () => {
    render(TooltipIconHost);

    const button = screen.getByRole('button', {name: '刷新'});
    expect(screen.getAllByRole('button', {name: '刷新'})).toHaveLength(1);
    await fireEvent.pointerEnter(button);
    expect(await screen.findByRole('tooltip')).toHaveTextContent('重新加载状态');
  });
});
