import {fireEvent, render, screen} from '@testing-library/svelte';
import {createRawSnippet} from 'svelte';
import {describe, expect, it} from 'vitest';

import {AppShell, type NavigationItem} from './index.js';

const navigation: NavigationItem[] = [
  {id: 'overview', label: '概览'},
  {id: 'settings', label: '设置'},
];

describe('responsive layout', () => {
  it('keeps one top navigation at every width', async () => {
    let activePage: NavigationItem['id'] = 'overview';
    render(AppShell, {
      props: {
        navigation,
        activePage,
        onNavigate: (page) => activePage = page,
        content: createRawSnippet(() => ({render: () => '<span>页面内容</span>'})),
      },
    });

    expect(screen.getByTestId('app-shell')).toHaveClass('min-w-0');
    const navigationLandmarks = screen.getAllByRole('navigation', {name: '主导航'});
    expect(navigationLandmarks).toHaveLength(1);
    expect(navigationLandmarks[0]).toHaveClass('overflow-x-auto');
    expect(navigationLandmarks[0]?.querySelector('ul')).toHaveClass('justify-start');
    for (const button of navigationLandmarks[0]?.querySelectorAll('button') ?? []) {
      expect(button.querySelector('svg[aria-hidden="true"]')).not.toBeNull();
      expect(button.querySelector('span')?.textContent).toBeTruthy();
    }
    expect(screen.getByRole('button', {name: '概览'})).toHaveAttribute('aria-current', 'page');

    await fireEvent.click(screen.getByRole('button', {name: '设置'}));
    expect(activePage).toBe('settings');
  });

  it('contains navigation overflow in the top bar', () => {
    render(AppShell, {
      props: {
        navigation,
        activePage: 'overview',
        onNavigate: () => {},
        content: createRawSnippet(() => ({render: () => '<span>页面内容</span>'})),
      },
    });

    const tabBar = screen.getByRole('navigation', {name: '主导航'});
    expect(tabBar).toHaveClass('overflow-x-auto');

    const tabStrip = tabBar.querySelector('ul');
    expect(tabStrip).toHaveClass('flex', 'min-w-max');
    for (const tabItem of tabBar.querySelectorAll('li')) {
      expect(tabItem).toHaveClass('shrink-0');
    }
  });
});
