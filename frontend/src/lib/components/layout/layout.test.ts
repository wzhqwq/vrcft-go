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
        content: createRawSnippet(() => ({render: () => '页面内容'})),
      },
    });

    expect(screen.getByTestId('app-shell')).toHaveClass('min-w-0');
    expect(screen.getByRole('navigation')).toHaveAccessibleName('主导航');

    await fireEvent.click(screen.getAllByRole('button', {name: '设置'})[0]);
    expect(activePage).toBe('settings');
  });
});
