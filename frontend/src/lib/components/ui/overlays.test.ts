import {fireEvent, render, screen} from '@testing-library/svelte';
import {describe, expect, it} from 'vitest';

import {Collapsible, Dialog, Tabs, Tooltip} from './index.js';

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

  it('returns focus to the dialog trigger after Escape closes a controlled dialog', async () => {
    render(Dialog, {props: {triggerLabel: '打开', title: '连接设置'}});

    const trigger = screen.getByRole('button', {name: '打开'});
    trigger.focus();
    await fireEvent.click(trigger);
    expect(screen.getByRole('dialog', {name: '连接设置'})).toBeVisible();
    await fireEvent.keyDown(document, {key: 'Escape'});
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
});
