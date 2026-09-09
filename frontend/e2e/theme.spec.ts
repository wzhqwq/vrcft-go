import {expect, test} from '@playwright/test';
import {installWailsMocks} from './fixtures';

test('success, overlay and shadows use replaceable semantic tokens', async ({page}) => {
  await installWailsMocks(page);
  await page.goto('/');
  await page.evaluate(() => {
    document.documentElement.style.setProperty('--color-success', 'rgb(1 200 100)');
    document.documentElement.style.setProperty('--color-overlay', 'rgb(20 30 40)');
    document.documentElement.style.setProperty('--color-shadow', 'rgb(50 60 70)');
  });
  await page.getByRole('button', {name: '设置', exact: true}).click();
  await page.getByRole('textbox', {name: 'Avatar OSC 根目录'}).fill('C:\\theme');
  await page.getByRole('button', {name: '保存更改'}).click();
  await expect.soft(page.getByText('已保存，将在重启后生效')).toHaveCSS('color', 'rgb(1, 200, 100)');
  await page.getByRole('textbox', {name: 'Avatar OSC 根目录'}).fill('C:\\changed');
  await page.getByRole('button', {name: '诊断', exact: true}).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  const styles = await page.evaluate(() => {
    const sample = (color: string) => {
      const canvas = document.createElement('canvas');
      canvas.width = canvas.height = 1;
      const context = canvas.getContext('2d')!;
      context.fillStyle = color;
      context.fillRect(0, 0, 1, 1);
      return [...context.getImageData(0, 0, 1, 1).data];
    };
    const overlay = getComputedStyle(document.querySelector('[data-dialog-overlay]')!);
    const dialog = getComputedStyle(document.querySelector('[role="dialog"]')!);
    return {overlay: sample(overlay.backgroundColor), shadow: sample(dialog.getPropertyValue('--tw-shadow-color')), boxShadow: dialog.boxShadow};
  });
  for (const [actual, expected] of [[styles.overlay, [20, 30, 40, 166]], [styles.shadow, [50, 60, 70, 102]]] as const) {
    actual.forEach((value, index) => expect(Math.abs(value - expected[index]!)).toBeLessThanOrEqual(1));
  }
  expect(styles.boxShadow).not.toBe('none');
});
