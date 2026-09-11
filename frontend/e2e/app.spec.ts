import {expect, test} from '@playwright/test';

import {calls, emit, installWailsMocks} from './fixtures';

test.beforeEach(async ({page}) => {
  await installWailsMocks(page);
  await page.goto('/');
  await expect(page.getByRole('heading', {name: '概览', exact: true})).toBeVisible();
});

test('shows authoritative overview avatar and OSC target', async ({page}) => {
  await expect(page.getByText('Authoritative Avatar')).toBeVisible();
  await expect(page.getByLabel('当前 Avatar').getByText('avtr_authoritative')).toBeVisible();
  await expect(page.getByText('192.168.1.10:9000')).toBeVisible();
  await expect(page.getByText('自动发现')).toBeVisible();
});

test('searches plugins and isolates a pending toggle to its target card', async ({page}) => {
  await page.getByRole('button', {name: '插件'}).click();
  await page.getByRole('searchbox', {name: '搜索插件'}).fill('Eye');
  await expect(page.getByRole('heading', {name: 'Eye Tracker'})).toBeVisible();
  await expect(page.getByRole('heading', {name: 'Lip Tracker'})).toBeHidden();

  await page.evaluate(() => (window as unknown as {__vrcftAcceptance: {holdPluginMutation(id: string): void}}).__vrcftAcceptance.holdPluginMutation('eye'));
  await page.getByRole('switch', {name: '启用 Eye Tracker'}).click();
  await expect(page.getByRole('switch', {name: '启用 Eye Tracker'})).toBeDisabled();

  await page.getByRole('searchbox', {name: '搜索插件'}).fill('');
  await expect(page.getByRole('switch', {name: '启用 Lip Tracker'})).toBeEnabled();
  await page.evaluate(() => (window as unknown as {__vrcftAcceptance: {resolvePluginMutation(id: string): void}}).__vrcftAcceptance.resolvePluginMutation('eye'));
  await expect(page.getByRole('switch', {name: '启用 Eye Tracker'})).toHaveAttribute('data-state', 'unchecked');
  expect(await calls(page)).toContainEqual(['PluginsAPI.SetEnabled', 'eye', false]);
});

test('protects a dirty settings draft, restores dialog focus, saves, and shows restart status', async ({page}) => {
  await page.getByRole('button', {name: '设置'}).click();
  const oscRoot = page.getByRole('textbox', {name: 'Avatar OSC 根目录'});
  await oscRoot.fill('C:\\New OSC');
  await oscRoot.blur();
  await expect(page.getByRole('region', {name: '未保存的更改'})).toBeVisible();

  const overview = page.getByRole('button', {name: '概览'});
  await overview.click();
  const confirmation = page.getByRole('dialog');
  await expect(confirmation).toBeVisible();
  await expect.poll(() => confirmation.evaluate((dialog) => dialog.contains(document.activeElement))).toBe(true);
  await page.keyboard.press('Escape');
  await expect(overview).toBeFocused();
  await expect(page.getByRole('heading', {name: '设置', exact: true})).toBeVisible();
  await expect(oscRoot).toHaveValue('C:\\New OSC');
  await expect(page.getByRole('region', {name: '未保存的更改'})).toBeVisible();

  await overview.click();
  await expect(confirmation).toBeVisible();
  await confirmation.getByRole('button', {name: '放弃更改'}).click();
  await expect(page.getByRole('heading', {name: '概览', exact: true})).toBeVisible();
  await page.getByRole('button', {name: '设置'}).click();
  await expect(oscRoot).toHaveValue('C:\\New OSC');
  await expect(page.getByRole('region', {name: '未保存的更改'})).toBeVisible();

  await page.getByRole('button', {name: '保存更改'}).click();
  await expect(page.getByRole('status')).toHaveText('已保存，将在重启后生效');
  expect(await calls(page)).toContainEqual(['SettingsAPI.Save', 1, expect.anything()]);
});

test('keeps a dirty settings draft on conflict and offers confirmed reload', async ({page}) => {
  await page.getByRole('button', {name: '设置'}).click();
  const oscRoot = page.getByRole('textbox', {name: 'Avatar OSC 根目录'});
  await oscRoot.fill('C:\\Conflict OSC');
  await page.evaluate(() => (window as unknown as {__vrcftAcceptance: {conflictNextSave(): void}}).__vrcftAcceptance.conflictNextSave());
  await page.getByRole('button', {name: '保存更改'}).click();
  await expect(page.getByRole('heading', {name: '设置已在其他位置更新'})).toBeVisible();
  await expect(oscRoot).toHaveValue('C:\\Conflict OSC');
  await expect(page.getByRole('region', {name: '未保存的更改'})).toBeVisible();
  const getsBeforeReload = (await calls(page)).filter((call) => call[0] === 'SettingsAPI.Get').length;
  await page.getByRole('button', {name: '重新加载设置'}).click();
  await expect(page.getByRole('heading', {name: '重新加载设置'})).toBeVisible();
  await page.getByRole('button', {name: '确认重新加载'}).click();
  await expect(oscRoot).toHaveValue('C:\\Authoritative revision 2');
  await expect(page.getByRole('heading', {name: '设置已在其他位置更新'})).toBeHidden();
  await expect(page.getByRole('region', {name: '未保存的更改'})).toBeHidden();
  await expect.poll(async () => (await calls(page)).filter((call) => call[0] === 'SettingsAPI.Get').length).toBe(getsBeforeReload + 1);
});

test('keeps raw diagnostics input out of the page and copied summary', async ({page}) => {
  await page.getByRole('button', {name: '诊断'}).click();
  await expect(page.getByRole('heading', {name: '诊断', exact: true})).toBeVisible();
  await expect(page.getByLabel('Runtime')).toContainText('修订 1');
  await expect(page.getByText('192.168.1.10:9000')).toBeVisible();
  const pageBody = page.locator('body');
  for (const marker of ['RAW_CONFIG_PATH_DO_NOT_LEAK', 'RAW_PLAN_ERROR_DO_NOT_LEAK', 'RAW_PRIVATE_VALUE_DO_NOT_LEAK']) {
    await expect(pageBody).not.toContainText(marker);
  }

  await page.getByRole('button', {name: '复制诊断信息'}).click();
  const copiedText = () => page.evaluate(() => (window as unknown as {__vrcftAcceptance: {copiedText?: string}}).__vrcftAcceptance.copiedText ?? '');
  await expect.poll(copiedText).toContain('Runtime: ready');
  await expect.poll(copiedText).toContain('Avatar plan unavailable');
  for (const marker of ['RAW_CONFIG_PATH_DO_NOT_LEAK', 'RAW_PLAN_ERROR_DO_NOT_LEAK', 'RAW_PRIVATE_VALUE_DO_NOT_LEAK']) {
    await expect.poll(copiedText).not.toContain(marker);
  }
});

test('retains detailed startup diagnostics when runtime status decoding fails', async ({page}) => {
  await page.evaluate(() => {
    const api = window.go.main.RuntimeAPI;
    api.GetStatus = async () => ({revision: 2, updatedAt: '2026-09-01T00:00:00Z', application: {osc: null}});
    api.GetDiagnostics = async () => {
      const failure = {id: 'startup-42', time: '2026-09-01T00:00:00Z', level: 'ERROR', component: 'runtime', stage: 'backend_start', message: 'listen udp :9001: address already in use; token=PRIVATE_CREDENTIAL'};
      return {entries: [failure], failure, logPath: 'C:\\Users\\Tester\\AppData\\Roaming\\vrcft-go\\logs\\application.jsonl', diskError: ''};
    };
  });
  await emit(page, 'vrcft:v1:runtime-status');
  await page.getByRole('button', {name: '诊断', exact: true}).click();
  await expect(page.getByRole('heading', {name: '启动错误'})).toBeVisible();
  await expect(page.getByRole('region', {name: '最近日志内容'})).toContainText('address already in use');
  await expect(page.locator('body')).not.toContainText('PRIVATE_CREDENTIAL');
  await expect(page.getByRole('article', {name: 'Runtime', exact: true})).toContainText('解析');
  await page.getByRole('button', {name: '复制诊断信息'}).click();
  const copied = () => page.evaluate(() => (window as unknown as {__vrcftAcceptance: {copiedText: string}}).__vrcftAcceptance.copiedText);
  await expect.poll(copied).toContain('startup-42');
  await expect.poll(copied).toContain('backend_start');
  await expect.poll(copied).not.toContain('PRIVATE_CREDENTIAL');
});

test('moves internal settings tabs with the keyboard', async ({page}) => {
  await page.getByRole('button', {name: '设置'}).click();
  const general = page.getByRole('tab', {name: '常规'});
  await general.focus();
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('tab', {name: '处理'})).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByRole('heading', {name: '默认通道处理'})).toBeVisible();
});

test('refreshes exactly the module that owns each Wails event', async ({page}) => {
  const before = await calls(page);
  const count = (name: string) => before.filter((call) => call[0] === name).length;
  const runtimeBefore = count('RuntimeAPI.GetStatus');
  const pluginsBefore = count('PluginsAPI.List');
  const settingsBefore = count('SettingsAPI.Get');

  await emit(page, 'vrcft:v1:plugins-changed');
  await expect.poll(async () => (await calls(page)).filter((call) => call[0] === 'PluginsAPI.List').length).toBe(pluginsBefore + 1);
  await emit(page, 'vrcft:v1:runtime-status');
  await expect.poll(async () => (await calls(page)).filter((call) => call[0] === 'RuntimeAPI.GetStatus').length).toBe(runtimeBefore + 1);
  await emit(page, 'vrcft:v1:settings-changed');
  await expect.poll(async () => (await calls(page)).filter((call) => call[0] === 'SettingsAPI.Get').length).toBe(settingsBefore + 1);

  const after = await calls(page);
  expect(after.filter((call) => call[0] === 'RuntimeAPI.GetStatus')).toHaveLength(runtimeBefore + 1);
  expect(after.filter((call) => call[0] === 'PluginsAPI.List')).toHaveLength(pluginsBefore + 1);
  expect(after.filter((call) => call[0] === 'SettingsAPI.Get')).toHaveLength(settingsBefore + 1);
});
