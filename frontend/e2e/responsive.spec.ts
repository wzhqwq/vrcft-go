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
    await page.getByRole('button', {name: '插件'}).click();
    await expect(page.getByRole('searchbox', {name: '搜索插件'})).toBeVisible();
    await page.getByRole('switch', {name: '启用 Eye Tracker'}).click();
    await expect(page.getByRole('switch', {name: '启用 Eye Tracker'})).toHaveAttribute('data-state', 'unchecked');
    await expectNoHorizontalOverflow(page);
  });
}
