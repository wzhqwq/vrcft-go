import {expect, test} from '@playwright/test'
import {installWailsMocks, viewports} from './fixtures'

async function expectNoHorizontalOverflow(page: import('@playwright/test').Page) {
  await expect.poll(() => page.evaluate(() => {
    const app = document.querySelector('[data-testid="app-shell"]')
    const regions = [...document.querySelectorAll('main, main section')]
    return document.documentElement.scrollWidth <= document.documentElement.clientWidth
      && (app === null || app.scrollWidth <= app.clientWidth)
      && regions.every((region) => region.scrollWidth <= region.clientWidth)
  })).toBe(true)
}

for (const viewport of viewports) {
  test(`keeps overview usable at ${viewport.width}x${viewport.height}`, async ({page}) => {
    await page.setViewportSize(viewport)
    await installWailsMocks(page)
    await page.goto('/')
    const navigation = page.locator('nav[aria-label="主导航"]')
    const title = page.getByRole('heading', {name: '概览', exact: true})
    await expect(navigation).toHaveCount(1)
    await expect(navigation).toBeVisible()
    await expect(title).toBeVisible()
    const navBox = await navigation.boundingBox()
    const titleBox = await title.boundingBox()
    expect((navBox?.y ?? 0) + (navBox?.height ?? 0)).toBeLessThanOrEqual(titleBox?.y ?? 0)

    const avatar = page.locator('section[aria-labelledby="avatar-drive-title"]')
    const plugins = page.locator('section[aria-labelledby="plugin-overview-title"]')
    const [avatarBox, pluginBox] = await Promise.all([avatar.boundingBox(), plugins.boundingBox()])
    if (viewport.width >= 1024) {
      expect(Math.abs(avatarBox!.y - pluginBox!.y)).toBeLessThan(1)
      expect(pluginBox!.x).toBeGreaterThan(avatarBox!.x + avatarBox!.width)
    } else {
      expect(pluginBox!.y).toBeGreaterThan(avatarBox!.y)
    }
    await expectNoHorizontalOverflow(page)

    await page.getByRole('button', {name: '查看参数列表'}).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    const dialogBox = await dialog.boundingBox()
    expect(dialogBox!.x).toBeGreaterThanOrEqual(0)
    expect(dialogBox!.x + dialogBox!.width).toBeLessThanOrEqual(viewport.width)
    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
    await expectNoHorizontalOverflow(page)
  })
}
