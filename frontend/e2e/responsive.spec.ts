import {expect, test} from '@playwright/test';

import {installWailsMocks, viewports} from './fixtures';

async function expectNoHorizontalOverflow(page: import('@playwright/test').Page) {
  await expect.poll(() => page.evaluate(() => {
    const app = document.querySelector('[data-testid="app-shell"]');
    const regions = [...document.querySelectorAll('main, main section, main article, [role="region"]')];
    return document.documentElement.scrollWidth <= document.documentElement.clientWidth
      && (app === null || app.scrollWidth <= app.clientWidth)
      && regions.every((region) => region.scrollWidth <= region.clientWidth);
  })).toBe(true);
}

for (const viewport of viewports) {
  test(`keeps ${viewport.width}x${viewport.height} structurally responsive and operable`, async ({page}) => {
    await page.setViewportSize(viewport);
    await installWailsMocks(page);
    await page.goto('/');
    await expect(page.getByRole('heading', {name: '概览', exact: true})).toBeVisible();

    const navigation = page.locator('nav[aria-label="主导航"]');
    const rail = navigation.nth(0);
    const topTabs = navigation.nth(1);
    const title = page.getByRole('heading', {name: '概览', exact: true});

    if (viewport.width === 640) {
      await expect(rail).toBeHidden();
      await expect(topTabs).toBeVisible();
      const [tabBox, titleBox] = await Promise.all([topTabs.boundingBox(), title.boundingBox()]);
      expect(tabBox).not.toBeNull();
      expect(titleBox).not.toBeNull();
      expect((tabBox?.y ?? 0) + (tabBox?.height ?? 0)).toBeLessThanOrEqual(titleBox?.y ?? 0);
    } else {
      await expect(rail).toBeVisible();
      await expect(topTabs).toBeHidden();
    }

    await expectNoHorizontalOverflow(page);
    const avatar = page.getByRole('region', {name: '当前 Avatar'});
    const osc = page.getByRole('region', {name: 'OSC 输出', exact: true});
    const [avatarBox, oscBox] = await Promise.all([avatar.boundingBox(), osc.boundingBox()]);
    if (viewport.width >= 1024) {
      expect(Math.abs(avatarBox!.y - oscBox!.y)).toBeLessThan(1);
      expect(oscBox!.x).toBeGreaterThan(avatarBox!.x + avatarBox!.width);
    } else {
      expect(oscBox!.y).toBeGreaterThan(avatarBox!.y);
    }
    await page.getByRole('button', {name: '插件'}).click();
    await expect(page.getByRole('searchbox', {name: '搜索插件'})).toBeVisible();
    await page.getByRole('switch', {name: '启用 Eye Tracker'}).click();
    await expect(page.getByRole('switch', {name: '启用 Eye Tracker'})).toHaveAttribute('data-state', 'unchecked');
    await expectNoHorizontalOverflow(page);
    const pluginGrid = page.locator('article').first().locator('..');
    await expect.poll(() => pluginGrid.evaluate((element) => getComputedStyle(element).gridTemplateColumns.split(' ').length)).toBe(viewport.width >= 1440 ? 3 : viewport.width >= 1024 ? 2 : 1);
    await page.getByRole('button', {name: '设置', exact: true}).click();
    for (const tab of ['常规', '处理', 'OSC']) {
      await page.getByRole('tab', {name: tab, exact: true}).click();
      if (tab === '处理') {
        const neutral = page.locator('#default-channel').getByRole('spinbutton', {name: '中立值', exact: true});
        const minimum = page.locator('#default-channel').getByRole('spinbutton', {name: '最小值', exact: true});
        const [neutralBox, minimumBox] = await Promise.all([neutral.boundingBox(), minimum.boundingBox()]);
        if (viewport.width >= 1024) {
          expect(Math.abs(neutralBox!.y - minimumBox!.y)).toBeLessThan(1);
          expect(minimumBox!.x).toBeGreaterThan(neutralBox!.x);
        } else expect(minimumBox!.y).toBeGreaterThan(neutralBox!.y);
      }
      await expectNoHorizontalOverflow(page);
      const controls = await page.locator('main input:visible').evaluateAll((inputs) => inputs.every((input) => {
        const box = input.getBoundingClientRect();
        const parent = input.parentElement!.getBoundingClientRect();
        return box.width > 100 && box.width <= parent.width;
      }));
      expect(controls).toBe(true);
    }
    const select = page.getByRole('button', {name: '目标模式'});
    await select.focus();
    await select.press('Enter');
    await expect(page.getByRole('listbox')).toBeVisible();
    const listBox = await page.getByRole('listbox').boundingBox();
    expect(listBox!.x).toBeGreaterThanOrEqual(0);
    expect(listBox!.x + listBox!.width).toBeLessThanOrEqual(viewport.width);
    await page.keyboard.press('Escape');
    await page.getByRole('tab', {name: '常规', exact: true}).click();
    await page.getByRole('textbox', {name: 'Avatar OSC 根目录'}).fill('C:\\responsive');
    await page.getByRole('button', {name: '诊断', exact: true}).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    const dialogBox = await dialog.boundingBox();
    expect(dialogBox!.x).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.x + dialogBox!.width).toBeLessThanOrEqual(viewport.width);
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
    await page.getByRole('region', {name: '未保存的更改'}).getByRole('button', {name: '放弃更改'}).click();
    await page.getByRole('button', {name: '诊断', exact: true}).click();
    await expect(page.getByRole('heading', {name: '诊断', exact: true})).toBeVisible();
    const moduleGrid = page.getByRole('article', {name: 'Runtime', exact: true}).locator('..');
    const rows = moduleGrid.getByRole('article');
    await expect(rows).toHaveCount(3);
    const firstRow = await rows.nth(0).boundingBox();
    const lastRow = await rows.nth(2).boundingBox();
    expect(lastRow!.y).toBeGreaterThan(firstRow!.y);
    expect(lastRow!.y + lastRow!.height - firstRow!.y).toBeLessThan(250);
    await page.getByRole('button', {name: '复制诊断信息', exact: true}).click();
    await expectNoHorizontalOverflow(page);
    await expect(page.getByRole('main')).toHaveCount(1);
    if (viewport.width === 1024) await page.screenshot({path: test.info().outputPath('diagnostics.png'), fullPage: true});
  });
}
