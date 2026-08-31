import {render, screen} from '@testing-library/svelte';
import {beforeAll, describe, expect, it} from 'vitest';

let ComponentWorkbench: typeof import('./ComponentWorkbench.svelte').default;

describe('ComponentWorkbench', () => {
  beforeAll(async () => {
    Object.assign(globalThis, {__COMPONENT_WORKBENCH__: true});
    ({default: ComponentWorkbench} = await import('./ComponentWorkbench.svelte'));
  });

  it('catalogues normal, focus, disabled, loading, empty, error, long-text, and narrow fixtures', () => {
    render(ComponentWorkbench);

    for (const fixture of ['常规', '焦点', '已禁用', '加载中', '空状态', '错误', '长文本', '320px 容器']) {
      expect(screen.getByRole('heading', {name: fixture})).toBeVisible();
    }
    expect(screen.getByTestId('workbench-320px')).toHaveClass('w-80');
  });
});
