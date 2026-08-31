import {fireEvent, render, screen} from '@testing-library/svelte';
import {createRawSnippet} from 'svelte';
import {describe, expect, it} from 'vitest';

import {AppShell, type NavigationItem} from './index.js';

const navigation: NavigationItem[] = [
  {id: 'overview', label: '概览'},
  {id: 'settings', label: '设置'},
];

describe('responsive layout', () => {
  it('provides an elastic shell with accessible primary navigation', async () => {
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
    expect(navigationLandmarks).toHaveLength(2);
    expect(navigationLandmarks[0]).toHaveClass('hidden', 'nav:flex');
    expect(navigationLandmarks[1]).toHaveClass('nav:hidden');

    await fireEvent.click(screen.getAllByRole('button', {name: '设置'})[0]);
    expect(activePage).toBe('settings');
  });

  it('grows narrow tab items evenly until their readable width requires scrolling', () => {
    render(AppShell, {
      props: {
        navigation,
        activePage: 'overview',
        onNavigate: () => {},
        content: createRawSnippet(() => ({render: () => '<span>页面内容</span>'})),
      },
    });

    const tabBar = screen.getAllByRole('navigation', {name: '主导航'})[1];
    expect(tabBar).toHaveClass('overflow-x-auto');

    const tabStrip = tabBar.querySelector('ul');
    expect(tabStrip).toHaveClass('flex', 'w-full', 'min-w-max');
    for (const tabItem of tabBar.querySelectorAll('li')) {
      expect(tabItem).toHaveClass('flex-1', 'min-w-32');
    }
  });
});
