import {fireEvent, render, screen} from '@testing-library/svelte';
import {describe, expect, it} from 'vitest';

import {Badge, Button, IconButton, Separator, Spinner} from './index.js';

describe('basic controls', () => {
  it('prevents an action while disabled or loading and announces loading state', () => {
    render(Button, {props: {label: '保存', loading: true, loadingLabel: '正在保存'}});

    expect(screen.getByRole('button', {name: '保存'})).toBeDisabled();
    expect(screen.getByRole('status')).toHaveTextContent('正在保存');
  });

  it('uses the supplied icon-button label as its accessible name', () => {
    render(IconButton, {props: {label: '关闭'}});

    expect(screen.getByRole('button', {name: '关闭'})).toBeEnabled();
  });

  it('forwards native button actions and attributes', async () => {
    let activations = 0;
    render(Button, {
      props: {label: '保存', name: 'save', value: 'settings', 'data-purpose': 'persist', onclick: () => activations += 1},
    });

    const button = screen.getByRole('button', {name: '保存'});
    expect(button).toHaveAttribute('name', 'save');
    expect(button).toHaveAttribute('value', 'settings');
    expect(button).toHaveAttribute('data-purpose', 'persist');
    await fireEvent.click(button);
    expect(activations).toBe(1);
  });

  it('forwards native icon-button actions without changing its accessible label', async () => {
    let activations = 0;
    render(IconButton, {props: {label: '刷新', title: '重新加载', onclick: () => activations += 1}});

    const button = screen.getByRole('button', {name: '刷新'});
    expect(button).toHaveAttribute('title', '重新加载');
    await fireEvent.click(button);
    expect(activations).toBe(1);
  });

  it('exposes textual status and decorative separators without relying on colour', () => {
    render(Badge, {props: {label: '连接失败', tone: 'danger'}});
    render(Spinner, {props: {label: '正在连接'}});
    render(Separator, {props: {decorative: true}});

    expect(screen.getByText('连接失败')).toBeVisible();
    expect(screen.getByRole('status')).toHaveTextContent('正在连接');
    expect(screen.queryByRole('separator')).not.toBeInTheDocument();
  });
});
