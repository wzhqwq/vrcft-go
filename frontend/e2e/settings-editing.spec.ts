import {expect, test} from '@playwright/test';
import {installWailsMocks} from './fixtures';

test.beforeEach(async ({page}) => {
  await page.setViewportSize({width: 640, height: 480});
  await installWailsMocks(page);
  await page.goto('/');
  await page.getByRole('button', {name: '设置', exact: true}).click();
});

for (const editor of ['paths', 'overrides', 'groups'] as const) {
  test(`${editor} preserves continuous text, focused control identity, and surviving row IDs`, async ({page}) => {
    if (editor !== 'paths') await page.getByRole('tab', {name: '处理', exact: true}).click();
    const label = editor === 'paths' ? '开发目录 0' : editor === 'overrides' ? '覆盖通道名称 0' : '互斥组 0 成员';
    const field = page.getByRole('textbox', {name: label, exact: true});
    const original = await field.inputValue();
    const id = await field.getAttribute('id');
    await field.focus();
    await field.press('End');
    const suffix = editor === 'groups' ? ', NEW' : 'XYZ';
    await page.keyboard.type(suffix, {delay: 25});
    await expect(field).toHaveValue(original + suffix);
    await expect(field).toBeFocused();
    await expect(field).toHaveAttribute('id', id!);
    const add = editor === 'paths' ? '添加开发目录' : editor === 'overrides' ? '添加通道覆盖' : '添加互斥组';
    const secondLabel = editor === 'paths' ? '开发目录 1' : editor === 'overrides' ? '覆盖通道名称 1' : '互斥组 1 成员';
    await page.getByRole('button', {name: add, exact: true}).click();
    const second = page.getByRole('textbox', {name: secondLabel, exact: true});
    await second.fill(editor === 'groups' ? 'left, right' : 'second');
    const secondId = await second.getAttribute('id');
    const remove = editor === 'paths' ? '删除开发目录 0' : editor === 'overrides' ? '删除通道覆盖 0' : '删除互斥组 0';
    await page.getByRole('button', {name: remove, exact: true}).click();
    await expect(page.getByRole('textbox', {name: label, exact: true})).toHaveAttribute('id', secondId!);
  });
}

test('initial Go default mutualExclusion null still exposes an editable Settings form', async ({page}) => {
  await page.addInitScript(() => {
    const get = window.go.main.SettingsAPI.Get;
    window.go.main.SettingsAPI.Get = async () => {
      const result = await get();
      result.settings.processing.mutualExclusion = null;
      return result;
    };
  });
  await page.reload();
  await page.getByRole('button', {name: '设置', exact: true}).click();
  await expect(page.getByRole('textbox', {name: 'Avatar OSC 根目录'})).toBeEditable();
  await page.getByRole('tab', {name: '处理', exact: true}).click();
  await page.getByRole('button', {name: '添加互斥组', exact: true}).click();
  await expect(page.getByRole('textbox', {name: '互斥组 0 成员', exact: true})).toBeEditable();
});
