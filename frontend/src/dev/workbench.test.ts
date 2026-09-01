import {render, screen} from '@testing-library/svelte';
import {beforeAll, describe, expect, it} from 'vitest';

let ComponentWorkbench: typeof import('./ComponentWorkbench.svelte').default;

describe('ComponentWorkbench', () => {
  beforeAll(async () => {
    ({default: ComponentWorkbench} = await import('./ComponentWorkbench.svelte'));
  });

  it('catalogues normal, focus, disabled, loading, empty, error, long-text, and narrow fixtures', () => {
    render(ComponentWorkbench);

    for (const fixture of ['常规', '焦点', '已禁用', '加载中', '空状态', '错误', '长文本', '320px 容器']) {
      expect(screen.getByRole('heading', {name: fixture})).toBeVisible();
    }
    expect(screen.getByTestId('workbench-320px')).toHaveClass('w-80');
    expect(screen.getByRole('region', {name: '工作台表单'})).toBeVisible();
    expect(screen.getByRole('textbox', {name: '本地地址'})).toBeVisible();
    expect(screen.getByText('输出端口')).toBeVisible();
  });
});
